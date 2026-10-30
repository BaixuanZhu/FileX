//go:build windows

package console

import (
	"os"

	"golang.org/x/sys/windows"
)

// openTerminal opens CONIN$, the console input buffer attached to this process.
// Redirected stdin does not affect what it reads, which is the property the
// confirmation gate needs: an agent piping "y" cannot reach this handle.
// It fails when the process has no console at all — a service, or a child
// spawned with pipes only — and callers read that failure as "not interactive".
func openTerminal() (*os.File, error) {
	name, err := windows.UTF16PtrFromString("CONIN$")
	if err != nil {
		return nil, err
	}

	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(handle), "CONIN$"), nil
}
