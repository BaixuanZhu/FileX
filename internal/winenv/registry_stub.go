//go:build !windows

package winenv

import (
	"errors"
	"runtime"
)

var errNotWindows = errors.New("windows registry operations not supported on " + runtime.GOOS)

func ReadStringValue(s Scope, name string) (value string, kind ValueKind, err error) {
	if s == ScopeProcess {
		return "", KindSZ, errNotWindows
	}
	return "", 0, errNotWindows
}

func WriteStringValue(s Scope, name, value string, kind ValueKind) error {
	return errNotWindows
}

func ReadKind(s Scope, name string) (ValueKind, error) {
	return 0, errNotWindows
}

func DeleteValue(s Scope, name string) error {
	return errNotWindows
}

func ReadAllStringValues(s Scope) (map[string]string, map[string]ValueKind, error) {
	return nil, nil, errNotWindows
}

func IsElevated() bool {
	return false
}

func NotifyEnvironmentChange() error {
	return nil // No-op on non-Windows
}

func NotifyEnvironmentChangeTimeout(timeoutMillis uint32) error {
	return nil // No-op on non-Windows
}
