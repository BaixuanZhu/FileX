package main

import (
	"os"
	"strings"

	"filex/internal/errs"
	"filex/internal/ops"
	"filex/internal/winenv"
)

func runEnv(args []string) *errs.Error {
	if len(args) == 0 {
		return errs.WithHint(
			errs.New(errs.EUsage, "env requires a subcommand"),
			"usage: filex env <get|set|list|unset> [args...]",
		)
	}

	switch args[0] {
	case "get":
		return runEnvGet(args[1:])
	case "set":
		return runEnvSet(args[1:])
	case "list":
		return runEnvList(args[1:])
	case "unset":
		return runEnvUnset(args[1:])
	default:
		return errs.Newf(errs.EUsage, "unknown env subcommand: %s", args[0])
	}
}

func runEnvGet(args []string) *errs.Error {
	scope := winenv.ScopeUser
	var name string

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--scope" && i+1 < len(args):
			i++
			s, err := winenv.ParseScope(args[i])
			if err != nil {
				return errs.Newf(errs.EUsage, "%v", err)
			}
			scope = s
		case strings.HasPrefix(args[i], "--scope="):
			s, err := winenv.ParseScope(strings.TrimPrefix(args[i], "--scope="))
			if err != nil {
				return errs.Newf(errs.EUsage, "%v", err)
			}
			scope = s
		case !strings.HasPrefix(args[i], "-"):
			name = args[i]
		}
	}

	if name == "" {
		return errs.WithHint(errs.New(errs.EUsage, "variable name required"), "filex env get --scope=user NAME")
	}

	result, err := ops.EnvGet(scope, name)
	if err != nil {
		return err
	}
	if wErr := errs.WriteSuccess(os.Stdout, result); wErr != nil {
		return errs.Newf(errs.EIO, "write output: %v", wErr)
	}
	return nil
}

func runEnvSet(args []string) *errs.Error {
	scope := winenv.ScopeUser
	var name, value string
	preserveKind := false
	noBroadcast := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--scope" && i+1 < len(args):
			i++
			s, err := winenv.ParseScope(args[i])
			if err != nil {
				return errs.Newf(errs.EUsage, "%v", err)
			}
			scope = s
		case strings.HasPrefix(args[i], "--scope="):
			s, err := winenv.ParseScope(strings.TrimPrefix(args[i], "--scope="))
			if err != nil {
				return errs.Newf(errs.EUsage, "%v", err)
			}
			scope = s
		case args[i] == "--preserve-kind":
			preserveKind = true
		case args[i] == "--no-broadcast":
			noBroadcast = true
		case !strings.HasPrefix(args[i], "-"):
			if name == "" {
				name = args[i]
			} else if value == "" {
				value = args[i]
			}
		}
	}

	if name == "" {
		return errs.WithHint(errs.New(errs.EUsage, "variable name required"), "filex env set --scope=user NAME VALUE")
	}

	result, err := ops.EnvSet(ops.EnvSetParams{
		Scope:        scope,
		Name:         name,
		Value:        value,
		PreserveKind: preserveKind,
		NoBroadcast:  noBroadcast,
	})
	if err != nil {
		return err
	}
	if wErr := errs.WriteSuccess(os.Stdout, result); wErr != nil {
		return errs.Newf(errs.EIO, "write output: %v", wErr)
	}
	return nil
}

func runEnvList(args []string) *errs.Error {
	scope := winenv.ScopeUser

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--scope" && i+1 < len(args):
			i++
			s, err := winenv.ParseScope(args[i])
			if err != nil {
				return errs.Newf(errs.EUsage, "%v", err)
			}
			scope = s
		case strings.HasPrefix(args[i], "--scope="):
			s, err := winenv.ParseScope(strings.TrimPrefix(args[i], "--scope="))
			if err != nil {
				return errs.Newf(errs.EUsage, "%v", err)
			}
			scope = s
		}
	}

	result, err := ops.EnvList(scope)
	if err != nil {
		return err
	}
	if wErr := errs.WriteSuccess(os.Stdout, result); wErr != nil {
		return errs.Newf(errs.EIO, "write output: %v", wErr)
	}
	return nil
}

func runEnvUnset(args []string) *errs.Error {
	scope := winenv.ScopeUser
	var name string

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--scope" && i+1 < len(args):
			i++
			s, err := winenv.ParseScope(args[i])
			if err != nil {
				return errs.Newf(errs.EUsage, "%v", err)
			}
			scope = s
		case strings.HasPrefix(args[i], "--scope="):
			s, err := winenv.ParseScope(strings.TrimPrefix(args[i], "--scope="))
			if err != nil {
				return errs.Newf(errs.EUsage, "%v", err)
			}
			scope = s
		case !strings.HasPrefix(args[i], "-"):
			name = args[i]
		}
	}

	if name == "" {
		return errs.WithHint(errs.New(errs.EUsage, "variable name required"), "filex env unset --scope=user NAME")
	}

	result, err := ops.EnvUnset(scope, name)
	if err != nil {
		return err
	}
	if wErr := errs.WriteSuccess(os.Stdout, result); wErr != nil {
		return errs.Newf(errs.EIO, "write output: %v", wErr)
	}
	return nil
}

