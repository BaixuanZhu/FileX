package winenv

import "fmt"

type Scope int

const (
	ScopeProcess Scope = iota
	ScopeUser
	ScopeMachine
)

func (s Scope) String() string {
	switch s {
	case ScopeProcess:
		return "process"
	case ScopeUser:
		return "user"
	case ScopeMachine:
		return "machine"
	default:
		return "unknown"
	}
}

func (s Scope) KeyPath() string {
	switch s {
	case ScopeUser:
		return `HKEY_CURRENT_USER\Environment`
	case ScopeMachine:
		return `HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	default:
		return ""
	}
}

func ParseScope(s string) (Scope, error) {
	switch s {
	case "process":
		return ScopeProcess, nil
	case "user":
		return ScopeUser, nil
	case "machine":
		return ScopeMachine, nil
	default:
		return 0, fmt.Errorf("unknown scope: %q (valid: process, user, machine)", s)
	}
}

type ValueKind uint32

const (
	KindSZ       ValueKind = 1
	KindExpandSZ ValueKind = 2
)

func (k ValueKind) String() string {
	switch k {
	case KindSZ:
		return "REG_SZ"
	case KindExpandSZ:
		return "REG_EXPAND_SZ"
	default:
		return fmt.Sprintf("REG_UNKNOWN(%d)", k)
	}
}
