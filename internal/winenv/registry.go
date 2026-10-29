package winenv

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var ErrAccessDenied = errors.New("registry access denied")

func rootAndSub(s Scope) (registry.Key, string) {
	switch s {
	case ScopeUser:
		return registry.CURRENT_USER, `Environment`
	case ScopeMachine:
		return registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	default:
		return 0, ""
	}
}

func ReadStringValue(s Scope, name string) (value string, kind ValueKind, err error) {
	if s == ScopeProcess {
		v, ok := os.LookupEnv(name)
		if !ok {
			return "", KindSZ, fmt.Errorf("variable %q not found in process scope", name)
		}
		return v, KindSZ, nil
	}

	root, sub := rootAndSub(s)
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", s.KeyPath(), err)
	}
	defer k.Close()

	_, typ, err := k.GetValue(name, nil)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return "", 0, fmt.Errorf("variable %q not found in %s scope", name, s)
		}
		return "", 0, fmt.Errorf("query %s\\%s: %w", s.KeyPath(), name, err)
	}

	vk := ValueKind(typ)
	if vk != KindSZ && vk != KindExpandSZ {
		return "", 0, fmt.Errorf("variable %q has unsupported type %v", name, vk)
	}

	str, _, err := k.GetStringValue(name)
	if err != nil {
		return "", 0, fmt.Errorf("read %s\\%s: %w", s.KeyPath(), name, err)
	}
	return str, vk, nil
}

func WriteStringValue(s Scope, name, value string, kind ValueKind) error {
	if s == ScopeProcess {
		return os.Setenv(name, value)
	}

	root, sub := rootAndSub(s)
	k, err := registry.OpenKey(root, sub, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("open %s for write: %w", s.KeyPath(), mapAccessErr(err))
	}
	defer k.Close()

	var setErr error
	if kind == KindExpandSZ {
		setErr = k.SetExpandStringValue(name, value)
	} else {
		setErr = k.SetStringValue(name, value)
	}
	if setErr != nil {
		return fmt.Errorf("write %s\\%s: %w", s.KeyPath(), name, mapAccessErr(setErr))
	}
	return nil
}

func ReadKind(s Scope, name string) (ValueKind, error) {
	if s == ScopeProcess {
		return KindSZ, nil
	}
	root, sub := rootAndSub(s)
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return 0, err
	}
	defer k.Close()
	_, typ, err := k.GetValue(name, nil)
	if err != nil {
		return 0, err
	}
	return ValueKind(typ), nil
}

func DeleteValue(s Scope, name string) error {
	if s == ScopeProcess {
		return os.Unsetenv(name)
	}

	root, sub := rootAndSub(s)
	k, err := registry.OpenKey(root, sub, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open %s for delete: %w", s.KeyPath(), mapAccessErr(err))
	}
	defer k.Close()

	if err := k.DeleteValue(name); err != nil {
		return fmt.Errorf("delete %s\\%s: %w", s.KeyPath(), name, err)
	}
	return nil
}

func ReadAllStringValues(s Scope) (map[string]string, map[string]ValueKind, error) {
	if s == ScopeProcess {
		vals := make(map[string]string)
		kinds := make(map[string]ValueKind)
		for _, e := range os.Environ() {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				vals[parts[0]] = parts[1]
				kinds[parts[0]] = KindSZ
			}
		}
		return vals, kinds, nil
	}

	root, sub := rootAndSub(s)
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", s.KeyPath(), err)
	}
	defer k.Close()

	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, nil, fmt.Errorf("enumerate %s: %w", s.KeyPath(), err)
	}

	vals := make(map[string]string, len(names))
	kinds := make(map[string]ValueKind, len(names))
	for _, name := range names {
		_, typ, getErr := k.GetValue(name, nil)
		if getErr != nil {
			continue
		}
		vk := ValueKind(typ)
		if vk != KindSZ && vk != KindExpandSZ {
			continue
		}
		str, _, strErr := k.GetStringValue(name)
		if strErr != nil {
			continue
		}
		vals[strings.ToUpper(name)] = str
		kinds[strings.ToUpper(name)] = vk
	}
	return vals, kinds, nil
}

func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func mapAccessErr(err error) error {
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%w: %w", ErrAccessDenied, err)
	}
	return err
}
