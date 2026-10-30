package journal_test

import (
	"strings"
	"testing"

	"filex/internal/journal"
)

func TestSessionFromEnv(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		t.Setenv("FILEX_SESSION_ID", "cursor-task-42")
		id, source := journal.SessionFromEnv()
		if id != "cursor-task-42" {
			t.Fatalf("id = %q, want the value unchanged", id)
		}
		if source != journal.SessionSourceEnv {
			t.Fatalf("source = %q, want %q", source, journal.SessionSourceEnv)
		}
	})

	// An orchestrator that does not opt in is not an error: the record just stays
	// unlabelled, which is what keeps old and new audit lines byte-identical.
	t.Run("unset", func(t *testing.T) {
		t.Setenv("FILEX_SESSION_ID", "")
		if id, source := journal.SessionFromEnv(); id != "" || source != "" {
			t.Fatalf("empty variable gave %q/%q, want no session", id, source)
		}
	})

	// Nothing that could break out of one JSON field gets in.
	for _, bad := range []string{
		"",
		"has space",
		"has/slash",
		"has\"quote",
		"has\nnewline",
		"has\ttab",
		"\x01control",
		"emoji-\U0001F600",
		strings.Repeat("x", 65),
	} {
		if journal.ValidSessionID(bad) {
			t.Fatalf("accepted %q", sanitizeForReport(bad))
		}
	}

	for _, good := range []string{"a", "task-42", "session_7", "v1.2", strings.Repeat("x", 64)} {
		if !journal.ValidSessionID(good) {
			t.Fatalf("rejected %q", good)
		}
	}
}

// An unusable id is dropped rather than repaired, so a malformed variable cannot
// turn into a truncated one that silently collides with a real session.
func TestSessionFromEnvRejectsUnusableValues(t *testing.T) {
	for _, bad := range []string{"has space", "has\nnewline", strings.Repeat("x", 200)} {
		t.Setenv("FILEX_SESSION_ID", bad)
		if id, source := journal.SessionFromEnv(); id != "" || source != "" {
			t.Fatalf("FILEX_SESSION_ID=%q produced %q/%q, want no session", sanitizeForReport(bad), id, source)
		}
	}
}

func TestNewSessionID(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := journal.NewSessionID()
		if !journal.ValidSessionID(id) {
			t.Fatalf("generated id %q would be rejected from the environment", id)
		}
		if len(id) != 32 {
			t.Fatalf("id %q is %d characters, want 32 hex", id, len(id))
		}
		if seen[id] {
			t.Fatalf("generated the same id twice: %q", id)
		}
		seen[id] = true
	}
}

// sanitizeForReport keeps control characters from making a failure unreadable.
func sanitizeForReport(s string) string {
	return strings.NewReplacer("\n", "\\n", "\t", "\\t", "\x01", "\\x01").Replace(s)
}
