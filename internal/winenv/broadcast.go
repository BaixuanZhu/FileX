package winenv

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	hwndBroadcast   = windows.HWND(0xFFFF)
	wmSettingChange = 0x001A
	smtoAbortIfHung = 0x0002
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	procSendMessageT = user32.NewProc("SendMessageTimeoutW")
)

func NotifyEnvironmentChange() error {
	return NotifyEnvironmentChangeTimeout(1500)
}

func NotifyEnvironmentChangeTimeout(timeoutMillis uint32) error {
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return fmt.Errorf("broadcast: %w", err)
	}
	var result uintptr
	if _, err := sendMessageTimeout(hwndBroadcast, wmSettingChange, 0,
		uintptr(unsafe.Pointer(env)), smtoAbortIfHung, timeoutMillis, &result); err != nil {
		return fmt.Errorf("broadcast: %w", err)
	}
	return nil
}

func sendMessageTimeout(hwnd windows.HWND, msg uint32, wparam, lparam uintptr, flags, timeoutMillis uint32, result *uintptr) (uintptr, error) {
	r1, _, callErr := procSendMessageT.Call(
		uintptr(hwnd), uintptr(msg), wparam, lparam,
		uintptr(flags), uintptr(timeoutMillis), uintptr(unsafe.Pointer(result)),
	)
	if r1 == 0 {
		return 0, fmt.Errorf("SendMessageTimeoutW: %w", callErr)
	}
	return r1, nil
}
