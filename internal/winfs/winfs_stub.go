//go:build !windows

package winfs

import (
	"errors"
	"os"
	"runtime"
)

var errNotWindows = errors.New("windows filesystem operations not supported on " + runtime.GOOS)

type LockStatus struct {
	Exclusive bool `json:"exclusive"`
	Deletable bool `json:"deletable"`
}

func CheckLock(path string) (LockStatus, error) {
	return LockStatus{}, errNotWindows
}

func AtomicReplace(src, dst string) error {
	return errNotWindows
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
