// Package console reaches the process's real interactive terminal, if it has
// one. The distinction matters: a redirected stdin may be an agent typing its
// own approval, while a terminal can only be a person.
package console

import (
	"errors"
	"os"
)

// ErrNoTerminal means no human can be reached from this process. Callers read it
// as "not interactive" and must not fall back to os.Stdin.
var ErrNoTerminal = errors.New("console: no interactive terminal for this process")

// OpenInput returns a readable handle on the real terminal.
//
// Two conditions are required, and the second is the one that is easy to miss:
// CONIN$ on Windows stays openable when stdin has been replaced by a pipe, so
// reading it would wait for a keystroke that an agent-driven invocation will
// never produce. Requiring stdin to be the terminal itself turns that wait into
// an immediate refusal.
func OpenInput() (*os.File, error) {
	if !stdinIsTerminal() {
		return nil, ErrNoTerminal
	}
	return openTerminal()
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// IsInteractive reports whether a human can be asked at this process's terminal.
func IsInteractive() bool {
	f, err := OpenInput()
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
