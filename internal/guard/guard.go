// Package guard decides whether a destructive action may run, without touching
// the outside world: Evaluate is a pure function so its table of decisions can
// be tested exhaustively. Confirmation I/O lives behind the Confirmer interface.
package guard

import (
	"path/filepath"
	"strings"

	"filex/internal/policy"
)

type Verb string

const (
	VerbRm       Verb = "rm"
	VerbWrite    Verb = "write"
	VerbEdit     Verb = "edit"
	VerbEnvSet   Verb = "env_set"
	VerbEnvUnset Verb = "env_unset"
	VerbUpgrade  Verb = "upgrade"
)

type Action struct {
	Verb  Verb
	Path  string // absolute
	Purge bool   // irreversible delete
	Scope string // env: process / user / machine
	Name  string // env variable name, for the audit trail
	Actor string // cli / mcp:<client>
	// ConfigRoot is the absolute path of ~/.filex, resolved by the caller.
	// Evaluate takes it as input so it stays free of I/O.
	ConfigRoot string
}

type Outcome string

const (
	OutcomeAllow        Outcome = "allow"
	OutcomeNeedsConfirm Outcome = "require_confirmation"
	OutcomeDeny         Outcome = "deny"
)

// Decision carries the structured answer the caller must both audit and return
// to whoever asked — reason and safer alternative are separate fields so an
// agent can act on them instead of parsing prose.
type Decision struct {
	Outcome          Outcome
	Class            string
	Reason           string
	SaferAlternative string
}

type Environment struct {
	Interactive bool
}

// Evaluate classifies an action. Four hardcoded classes, everything else is
// allowed — path policy already gates at the layer below.
func Evaluate(a Action, cfg policy.Guard, env Environment) Decision {
	if !cfg.IsEnabled() {
		return Decision{Outcome: OutcomeAllow, Class: "disabled"}
	}

	class, reason, alternative := classify(a)
	if class == "" {
		return Decision{Outcome: OutcomeAllow}
	}
	if env.Interactive {
		return Decision{Outcome: OutcomeNeedsConfirm, Class: class, Reason: reason, SaferAlternative: alternative}
	}
	return Decision{Outcome: OutcomeDeny, Class: class, Reason: reason, SaferAlternative: alternative}
}

const (
	ClassPurgeDelete = "purge_delete"
	ClassMachineEnv  = "machine_env_write"
	ClassSelfConfig  = "self_config_write"
	ClassSelfUpdate  = "self_update"
)

func classify(a Action) (class, reason, alternative string) {
	switch a.Verb {
	case VerbRm:
		if a.Purge {
			return ClassPurgeDelete,
				"permanent delete is unrecoverable",
				"drop --purge and the file moves to the trash dir, where it can be recovered"
		}
	case VerbEnvSet, VerbEnvUnset:
		if strings.EqualFold(a.Scope, "machine") {
			return ClassMachineEnv,
				"machine-scope environment changes affect every account on this host",
				"use --scope=user to change only the current account"
		}
	case VerbWrite, VerbEdit:
		if withinConfigRoot(a.Path, a.ConfigRoot) {
			return ClassSelfConfig,
				"writing into filex's own config directory can widen the limits that constrain it",
				"edit the file by hand in a terminal, or change the setting through a dedicated command"
		}
	case VerbUpgrade:
		return ClassSelfUpdate,
			"replacing the binary changes what filex itself does on every later call",
			"run filex upgrade --check to inspect first, or confirm in a terminal"
	}
	return "", "", ""
}

func withinConfigRoot(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) {
		return false
	}
	return policy.MatchesPattern(path, root+string(filepath.Separator)+"**")
}
