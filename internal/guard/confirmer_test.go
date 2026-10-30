package guard

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"filex/internal/console"
)

func TestPromptAcceptsOnlyYes(t *testing.T) {
	cases := []struct {
		name       string
		answer     string
		want       bool
		wantSource string
	}{
		{name: "y", answer: "y\n", want: true, wantSource: SourceTTY},
		{name: "capital Y", answer: "Y\n", want: true, wantSource: SourceTTY},
		{name: "padded yes", answer: "  yes  \n", want: true, wantSource: SourceTTY},
		{name: "n", answer: "n\n", want: false, wantSource: SourceTTY},
		// Bare enter is the documented default: no.
		{name: "enter", answer: "\n", want: false, wantSource: SourceTTY},
		{name: "not an answer", answer: "maybe\n", want: false, wantSource: SourceTTY},
		// A handle that closes before anyone answers is a refusal with no
		// confirmation to report.
		{name: "closed", answer: "", want: false, wantSource: SourceNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			confirmed, source, err := prompt(strings.NewReader(tc.answer), &out,
				Action{Verb: VerbRm, Path: `C:\work\victim.txt`, Purge: true},
				Decision{
					Outcome:          OutcomeNeedsConfirm,
					Class:            ClassPurgeDelete,
					Reason:           "permanent delete is unrecoverable",
					SaferAlternative: "drop --purge",
				})
			if err != nil {
				t.Fatalf("prompt: %v", err)
			}
			if confirmed != tc.want {
				t.Fatalf("answer %q: confirmed = %v, want %v", tc.answer, confirmed, tc.want)
			}
			if source != tc.wantSource {
				t.Fatalf("answer %q: source = %q, want %q", tc.answer, source, tc.wantSource)
			}

			text := out.String()
			// The person answering must be able to decide from what is on screen,
			// so the reason and the way out both have to appear.
			if !strings.Contains(text, "permanent delete is unrecoverable") {
				t.Errorf("prompt omitted the reason: %q", text)
			}
			if !strings.Contains(text, "drop --purge") {
				t.Errorf("prompt omitted the safer alternative: %q", text)
			}
			if !strings.Contains(text, "victim.txt") {
				t.Errorf("prompt did not name the file: %q", text)
			}
		})
	}
}

// An answer arriving on a failing stream is "no", not "retry": a confirmation that
// cannot be read must never default to running the operation.
func TestPromptReadErrorDenies(t *testing.T) {
	var out bytes.Buffer
	confirmed, source, err := prompt(errReader{}, &out, Action{Verb: VerbUpgrade}, Decision{Reason: "x"})
	if err != nil {
		t.Fatalf("prompt returned %v; a read failure must look like a refusal", err)
	}
	if confirmed {
		t.Fatal("confirmed on a failing reader")
	}
	if source != SourceNone {
		t.Fatalf("source = %q, want %q", source, SourceNone)
	}
}

type errReader struct{}

func (errReader) Read(p []byte) (int, error) { return 0, errors.New("closed") }

func TestConfirmerSources(t *testing.T) {
	a := Action{Verb: VerbRm, Purge: true}
	d := Decision{Outcome: OutcomeNeedsConfirm, Class: ClassPurgeDelete}

	denyConfirmed, denySource, denyErr := DenyConfirmer{}.Confirm(a, d)
	if denyConfirmed || denySource != SourceNone || denyErr != nil {
		t.Fatalf("DenyConfirmer = %v/%q/%v", denyConfirmed, denySource, denyErr)
	}

	bypassConfirmed, bypassSource, bypassErr := BypassConfirmer{}.Confirm(a, d)
	if !bypassConfirmed || bypassSource != SourceBypass || bypassErr != nil {
		t.Fatalf("BypassConfirmer = %v/%q/%v", bypassConfirmed, bypassSource, bypassErr)
	}
}

// The point of TTYConfirmer is that it will not read the caller's stdin. A test
// process has a piped stdin, so reaching for input at all would be the bug.
func TestTTYConfirmerNeverReadsPipedStdin(t *testing.T) {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice != 0 {
		t.Skip("this test only means something when stdin is not a terminal")
	}

	confirmed, source, err := TTYConfirmer{}.Confirm(
		Action{Verb: VerbRm, Purge: true},
		Decision{Outcome: OutcomeNeedsConfirm, Class: ClassPurgeDelete, Reason: "r"},
	)
	if err != nil {
		t.Fatalf("a missing terminal must be a refusal, not an error: %v", err)
	}
	if confirmed {
		t.Fatal("TTYConfirmer confirmed with no terminal")
	}
	if source != SourceNone {
		t.Fatalf("source = %q, want %q", source, SourceNone)
	}

	if _, err := console.OpenInput(); !errors.Is(err, console.ErrNoTerminal) {
		t.Fatalf("console.OpenInput() = %v, want ErrNoTerminal", err)
	}
}
