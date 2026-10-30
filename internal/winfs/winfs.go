//go:build windows

package winfs

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	modkernel32            = syscall.NewLazyDLL("kernel32.dll")
	procCreateFileW        = modkernel32.NewProc("CreateFileW")
	procMoveFileExW        = modkernel32.NewProc("MoveFileExW")
	procGetFileAttributesW = modkernel32.NewProc("GetFileAttributesW")
)

const (
	_GENERIC_READ            = 0x80000000
	_GENERIC_WRITE           = 0x40000000
	_DELETE                  = 0x00010000
	_FILE_SHARE_READ         = 0x00000001
	_FILE_SHARE_WRITE        = 0x00000002
	_FILE_SHARE_DELETE       = 0x00000004
	_OPEN_EXISTING           = 3
	_FILE_ATTRIBUTE_NORMAL   = 0x80
	_REPLACE_EXISTING        = 0x1
	_MOVEFILE_WRITE_THROUGH  = 0x8
	_INVALID_FILE_ATTRIBUTES = ^uintptr(0)
)

type LockStatus struct {
	Exclusive bool `json:"exclusive"`
	Deletable bool `json:"deletable"`
}

func CheckLock(path string) (LockStatus, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return LockStatus{}, err
	}

	p, err := syscall.UTF16PtrFromString(absPath)
	if err != nil {
		return LockStatus{}, err
	}

	exclusive := tryExclusive(p)
	deletable := tryDeletable(p)

	return LockStatus{
		Exclusive: exclusive,
		Deletable: deletable,
	}, nil
}

func tryExclusive(path *uint16) bool {
	handle, _, _ := procCreateFileW.Call(
		uintptr(unsafe.Pointer(path)),
		_GENERIC_READ|_GENERIC_WRITE,
		0,
		0,
		_OPEN_EXISTING,
		_FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if handle == uintptr(syscall.InvalidHandle) {
		return false
	}
	syscall.CloseHandle(syscall.Handle(handle))
	return true
}

func tryDeletable(path *uint16) bool {
	handle, _, _ := procCreateFileW.Call(
		uintptr(unsafe.Pointer(path)),
		_DELETE,
		_FILE_SHARE_READ|_FILE_SHARE_WRITE|_FILE_SHARE_DELETE,
		0,
		_OPEN_EXISTING,
		_FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if handle == uintptr(syscall.InvalidHandle) {
		return false
	}
	syscall.CloseHandle(syscall.Handle(handle))
	return true
}

func AtomicReplace(src, dst string) error {
	srcPath, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	dstPath, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	srcPtr, err := syscall.UTF16PtrFromString(srcPath)
	if err != nil {
		return err
	}
	dstPtr, err := syscall.UTF16PtrFromString(dstPath)
	if err != nil {
		return err
	}

	ret, _, err := procMoveFileExW.Call(
		uintptr(unsafe.Pointer(srcPtr)),
		uintptr(unsafe.Pointer(dstPtr)),
		_REPLACE_EXISTING|_MOVEFILE_WRITE_THROUGH,
	)
	if ret == 0 {
		return err
	}
	return nil
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
