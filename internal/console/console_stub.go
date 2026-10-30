//go:build !windows

package console

import "os"

// openTerminal opens the controlling terminal rather than stdin, for the same
// reason Windows opens CONIN$: a pipe must not be able to impersonate a person.
// No controlling terminal means the open fails and the caller is non-interactive.
func openTerminal() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDONLY, 0)
}
