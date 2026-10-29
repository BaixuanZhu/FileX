//go:build !windows

package winenv

import (
	"errors"
	"runtime"
)

var errNotWindows = errors.New("windows registry operations not supported on " + runtime.GOOS)

func GetEnv(name string) (string, error) {
	return "", errNotWindows
}

func SetEnv(name, value string, scope Scope) error {
	return errNotWindows
}

func UnsetEnv(name string, scope Scope) error {
	return errNotWindows
}

func ListEnv(scope Scope) (map[string]string, error) {
	return nil, errNotWindows
}
