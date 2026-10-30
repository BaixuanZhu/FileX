package ops

import (
	"os"
	"strings"

	"filex/internal/errs"
	"filex/internal/guard"
	"filex/internal/winenv"
)

// sensitivePatterns contains substrings that indicate a variable may hold secrets.
var sensitivePatterns = []string{
	"PASSWORD", "PASSWD", "PWD",
	"SECRET", "KEY", "TOKEN", "CREDENTIAL", "CRED",
	"PRIVATE", "SIGNATURE",
}

func isSensitiveName(name string) bool {
	upper := strings.ToUpper(name)
	for _, p := range sensitivePatterns {
		if strings.Contains(upper, p) {
			return true
		}
	}
	return false
}

const maskedValue = "********"

type EnvVar struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Expanded string `json:"expanded,omitempty"`
	Kind     string `json:"kind"`
	Scope    string `json:"scope"`
}

type EnvGetResult struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Expanded string `json:"expanded,omitempty"`
	Kind     string `json:"kind"`
	Scope    string `json:"scope"`
}

func EnvGet(scope winenv.Scope, name string) (*EnvGetResult, *errs.Error) {
	if name == "" {
		return nil, errs.WithHint(errs.New(errs.EUsage, "variable name is required"), "provide a variable name")
	}

	if scope == winenv.ScopeProcess {
		val, ok := os.LookupEnv(name)
		if !ok {
			return nil, errs.Newf(errs.ENotFound, "variable %q not found in process scope", name)
		}
		return &EnvGetResult{
			Name:  name,
			Value: val,
			Kind:  winenv.KindSZ.String(),
			Scope: scope.String(),
		}, nil
	}

	val, kind, err := winenv.ReadStringValue(scope, name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, errs.Newf(errs.ENotFound, "variable %q not found in %s scope", name, scope)
		}
		return nil, mapEnvErr(err)
	}

	expanded := val
	if kind == winenv.KindExpandSZ {
		sysVars, _, _ := winenv.ReadAllStringValues(winenv.ScopeMachine)
		usrVars, _, _ := winenv.ReadAllStringValues(winenv.ScopeUser)
		if sysVars == nil {
			sysVars = make(map[string]string)
		}
		if usrVars == nil {
			usrVars = make(map[string]string)
		}
		composed := winenv.ComposeVars(sysVars, usrVars)
		expanded = winenv.ExpandEnv(val, composed)
	}

	return &EnvGetResult{
		Name:     name,
		Value:    val,
		Expanded: expanded,
		Kind:     kind.String(),
		Scope:    scope.String(),
	}, nil
}

type EnvSetParams struct {
	Scope        winenv.Scope
	Name         string
	Value        string
	PreserveKind bool
	NoBroadcast  bool
}

type EnvSetResult struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Kind      string `json:"kind"`
	Scope     string `json:"scope"`
	Broadcast bool   `json:"broadcast"`
}

func EnvSet(p EnvSetParams) (*EnvSetResult, *errs.Error) {
	if p.Name == "" {
		return nil, errs.WithHint(errs.New(errs.EUsage, "variable name is required"), "provide a variable name")
	}

	if p.Scope == winenv.ScopeProcess {
		if err := os.Setenv(p.Name, p.Value); err != nil {
			return nil, errs.Newf(errs.EInternal, "set process env failed: %v", err)
		}
		return &EnvSetResult{
			Name:  p.Name,
			Value: p.Value,
			Kind:  winenv.KindSZ.String(),
			Scope: p.Scope.String(),
		}, nil
	}

	r, gErr := gate(guard.Action{Verb: guard.VerbEnvSet, Scope: p.Scope.String(), Name: p.Name})
	if gErr != nil {
		return nil, gErr
	}

	if p.Scope == winenv.ScopeMachine && !winenv.IsElevated() {
		return nil, errs.WithHint(
			errs.New(errs.EDenied, "setting machine-scope variables requires administrator privileges"),
			"run in an elevated terminal",
		)
	}

	kind := winenv.KindSZ
	if strings.Contains(p.Value, "%") {
		kind = winenv.KindExpandSZ
	}

	if p.PreserveKind {
		if existingKind, err := winenv.ReadKind(p.Scope, p.Name); err == nil {
			kind = existingKind
		}
	}

	if err := winenv.WriteStringValue(p.Scope, p.Name, p.Value, kind); err != nil {
		return nil, mapEnvErr(err)
	}

	broadcast := false
	if !p.NoBroadcast {
		if bErr := winenv.NotifyEnvironmentChange(); bErr == nil {
			broadcast = true
		}
	}

	recordEnv("env_set", p.Scope.String(), p.Name, p.Value, r)

	return &EnvSetResult{
		Name:      p.Name,
		Value:     p.Value,
		Kind:      kind.String(),
		Scope:     p.Scope.String(),
		Broadcast: broadcast,
	}, nil
}

type EnvListResult struct {
	Scope     string   `json:"scope"`
	Variables []EnvVar `json:"variables"`
}

func EnvList(scope winenv.Scope) (*EnvListResult, *errs.Error) {
	if scope == winenv.ScopeProcess {
		vars := make([]EnvVar, 0)
		for _, e := range os.Environ() {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				displayValue := parts[1]
				if isSensitiveName(parts[0]) {
					displayValue = maskedValue
				}
				vars = append(vars, EnvVar{
					Name:  parts[0],
					Value: displayValue,
					Kind:  winenv.KindSZ.String(),
					Scope: scope.String(),
				})
			}
		}
		return &EnvListResult{Scope: scope.String(), Variables: vars}, nil
	}

	vals, kinds, err := winenv.ReadAllStringValues(scope)
	if err != nil {
		return nil, mapEnvErr(err)
	}

	var sysVars, usrVars map[string]string
	if scope == winenv.ScopeUser {
		sysVals, _, _ := winenv.ReadAllStringValues(winenv.ScopeMachine)
		sysVars = sysVals
		usrVars = vals
	} else {
		sysVars = vals
		usrVals, _, _ := winenv.ReadAllStringValues(winenv.ScopeUser)
		usrVars = usrVals
	}
	composed := winenv.ComposeVars(sysVars, usrVars)

	vars := make([]EnvVar, 0, len(vals))
	seen := make(map[string]bool)
	for name, val := range vals {
		if seen[name] {
			continue
		}
		seen[name] = true
		expanded := val
		if kinds[name] == winenv.KindExpandSZ {
			expanded = winenv.ExpandEnv(val, composed)
		}
		displayValue := val
		displayExpanded := expanded
		if isSensitiveName(name) {
			displayValue = maskedValue
			displayExpanded = maskedValue
		}
		vars = append(vars, EnvVar{
			Name:     name,
			Value:    displayValue,
			Expanded: displayExpanded,
			Kind:     kinds[name].String(),
			Scope:    scope.String(),
		})
	}

	return &EnvListResult{Scope: scope.String(), Variables: vars}, nil
}

type EnvUnsetResult struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

func EnvUnset(scope winenv.Scope, name string) (*EnvUnsetResult, *errs.Error) {
	if name == "" {
		return nil, errs.WithHint(errs.New(errs.EUsage, "variable name is required"), "provide a variable name")
	}

	if scope == winenv.ScopeProcess {
		return nil, errs.WithHint(
			errs.New(errs.EUnsupported, "process-scope unset is not persisted"),
			"use set with empty value, or unset in the calling shell",
		)
	}

	r, gErr := gate(guard.Action{Verb: guard.VerbEnvUnset, Scope: scope.String(), Name: name})
	if gErr != nil {
		return nil, gErr
	}

	if scope == winenv.ScopeMachine && !winenv.IsElevated() {
		return nil, errs.WithHint(
			errs.New(errs.EDenied, "deleting machine-scope variables requires administrator privileges"),
			"run in an elevated terminal",
		)
	}

	if err := winenv.DeleteValue(scope, name); err != nil {
		if strings.Contains(err.Error(), "not found") ||
			strings.Contains(err.Error(), "The system cannot find the file specified") {
			return nil, errs.Newf(errs.ENotFound, "variable %q not found in %s scope", name, scope)
		}
		return nil, mapEnvErr(err)
	}

	_ = winenv.NotifyEnvironmentChange()

	recordEnv("env_unset", scope.String(), name, "", r)

	return &EnvUnsetResult{Name: name, Scope: scope.String()}, nil
}

func mapEnvErr(err error) *errs.Error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "access denied") || strings.Contains(msg, "Access is denied") {
		return errs.WithHint(
			errs.Newf(errs.EDenied, "%v", err),
			"run in an elevated terminal for scope writes",
		)
	}
	return errs.Newf(errs.EInternal, "%v", err)
}
