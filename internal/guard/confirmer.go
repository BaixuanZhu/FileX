package guard

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"filex/internal/console"
)

// Confirmer answers "did a human approve this?". The returned source is audited,
// so a person at a terminal and a flag on the command line stay distinguishable.
type Confirmer interface {
	Confirm(a Action, d Decision) (confirmed bool, source string, err error)
}

const (
	SourceTTY    = "tty"
	SourceNone   = "none"
	SourceBypass = "bypass:flag"
)

// DenyConfirmer is bound by MCP: a model asking a second time is not a human
// approving the first time.
type DenyConfirmer struct{}

func (DenyConfirmer) Confirm(a Action, d Decision) (bool, string, error) {
	return false, SourceNone, nil
}

// BypassConfirmer stands in for the deliberately verbose command-line flag. The
// entry point only installs it when the config allows bypass.
type BypassConfirmer struct{}

func (BypassConfirmer) Confirm(a Action, d Decision) (bool, string, error) {
	return true, SourceBypass, nil
}

// TTYConfirmer asks at the real terminal. It reads the console input handle,
// never os.Stdin — a pipe is exactly how an agent would answer on its own
// behalf, so falling back to stdin would defeat the whole check. console refuses
// to hand out a handle unless stdin really is that terminal, so an agent-driven
// call gets an immediate refusal instead of waiting for a keystroke nobody types.
type TTYConfirmer struct{}

func (TTYConfirmer) Confirm(a Action, d Decision) (bool, string, error) {
	in, err := console.OpenInput()
	if err != nil {
		return false, SourceNone, nil
	}
	defer in.Close()

	return prompt(in, os.Stderr, a, d)
}

func prompt(in io.Reader, out io.Writer, a Action, d Decision) (bool, string, error) {
	fmt.Fprintf(out, "\nfilex: %s\n", d.Reason)
	fmt.Fprintf(out, "       safer option: %s\n", d.SaferAlternative)
	fmt.Fprintf(out, "%s %s -- irreversible, confirm? [y/N] ", a.Verb, describe(a))
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && answer == "" {
		return false, SourceNone, nil
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, SourceTTY, nil
	}
	return false, SourceTTY, nil
}

func describe(a Action) string {
	switch {
	case a.Path != "":
		return a.Path
	case a.Scope != "":
		return "scope=" + a.Scope
	default:
		return ""
	}
}
