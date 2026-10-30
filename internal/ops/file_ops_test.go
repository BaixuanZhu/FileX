package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filex/internal/errs"
	"filex/internal/guard"
)

// isolate points HOME, USERPROFILE and the working directory at a temp dir, so
// policy checks, trash and journal all stay inside the test.
func isolate(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}
	t.Setenv("HOME", resolved)
	t.Setenv("USERPROFILE", resolved)
	t.Chdir(resolved)

	return resolved
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
}

// bindRuntime installs a runtime for one test and puts the previous one back.
// Operations read the package-level runtime, so tests that do not bind one get
// the fail-closed default — which is itself the behaviour worth asserting.
func bindRuntime(t *testing.T, r Runtime) {
	t.Helper()
	before := rt
	BindRuntime(r)
	t.Cleanup(func() { rt = before })
}

// humanAtTerminal is the test stand-in for someone answering a prompt. It
// records the actions it was asked about, so a test can assert that a
// confirmation was actually sought rather than assumed.
type humanAtTerminal struct {
	confirmed bool
	asked     []guard.Action
}

func (h *humanAtTerminal) Confirm(a guard.Action, d guard.Decision) (bool, string, error) {
	h.asked = append(h.asked, a)
	if h.confirmed {
		return true, guard.SourceTTY, nil
	}
	return false, guard.SourceNone, nil
}

func TestRmDryRunChangesNothing(t *testing.T) {
	root := isolate(t)
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	before, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat before: %v", statErr)
	}

	result, rmErr := Rm(RmParams{Path: path, DryRun: true})
	if rmErr != nil {
		t.Fatalf("dry run returned error: %v", rmErr)
	}

	if !result.DryRun || result.Deleted || result.Purged {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Action != "move_to_trash" {
		t.Fatalf("action = %q, want move_to_trash", result.Action)
	}
	if result.WillMoveTo == "" {
		t.Fatal("dry run must report where the file would go")
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file vanished on dry run: %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("mtime changed: %v -> %v", before.ModTime(), after.ModTime())
	}
	if _, err := os.Stat(filepath.Join(root, ".filex", "trash")); !os.IsNotExist(err) {
		t.Fatalf("dry run created a trash directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".filex", "journal", "filex.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote a journal entry: %v", err)
	}
}

func TestRmDryRunPurgeReportsPermanentDelete(t *testing.T) {
	root := isolate(t)
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	result, rmErr := Rm(RmParams{Path: path, Purge: true, DryRun: true})
	if rmErr != nil {
		t.Fatalf("dry run returned error: %v", rmErr)
	}

	if result.Action != "permanent_delete" {
		t.Fatalf("action = %q, want permanent_delete", result.Action)
	}
	if result.Deleted || !result.DryRun {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("purge dry run removed the file: %v", err)
	}
}

func TestRmMovesToTrash(t *testing.T) {
	root := isolate(t)
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	result, rmErr := Rm(RmParams{Path: path})
	if rmErr != nil {
		t.Fatalf("rm: %v", rmErr)
	}

	if !result.Deleted || result.Purged || result.DryRun {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("original still present: %v", err)
	}
	if _, err := os.Stat(result.Trash); err != nil {
		t.Fatalf("moved copy missing: %v", err)
	}
}

func TestRmPurgeDeletesForReal(t *testing.T) {
	root := isolate(t)
	bindRuntime(t, Runtime{Actor: "test", Confirmer: guard.BypassConfirmer{}})
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	result, rmErr := Rm(RmParams{Path: path, Purge: true})
	if rmErr != nil {
		t.Fatalf("rm --purge: %v", rmErr)
	}

	if !result.Deleted || !result.Purged {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("purge left the file: %v", err)
	}
}

// The default runtime has no way to reach a human, so an irreversible delete
// must stop there rather than proceed. This is the property the whole gate
// exists for: a caller that never binds, or binds without a terminal, gets the
// safest answer.
func TestRmPurgeDeniedWithoutConfirmation(t *testing.T) {
	root := isolate(t)
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	_, rmErr := Rm(RmParams{Path: path, Purge: true})
	if rmErr == nil {
		t.Fatal("purge succeeded with no human confirming it")
	}
	if rmErr.Code != errs.EDenied {
		t.Fatalf("code = %s, want E_DENIED", rmErr.Code)
	}
	if rmErr.GuardClass != guard.ClassPurgeDelete {
		t.Fatalf("class = %q, want %q", rmErr.GuardClass, guard.ClassPurgeDelete)
	}
	if rmErr.Reason == "" || rmErr.SaferAlternative == "" {
		t.Fatalf("denial must carry both a reason and an alternative: %+v", rmErr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("denied purge removed the file: %v", err)
	}
}

// A trash move is recoverable, so it is not a guarded action: it must not be
// held up by a confirmation prompt that would train people to answer "y" to
// everything.
func TestRmToTrashNeedsNoConfirmation(t *testing.T) {
	root := isolate(t)
	h := &humanAtTerminal{}
	bindRuntime(t, Runtime{Actor: "test", Interactive: true, Confirmer: h})
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	if _, rmErr := Rm(RmParams{Path: path}); rmErr != nil {
		t.Fatalf("rm: %v", rmErr)
	}
	if len(h.asked) != 0 {
		t.Fatalf("recoverable delete asked for confirmation: %+v", h.asked)
	}
}

func TestRmAsksBeforePurge(t *testing.T) {
	root := isolate(t)
	h := &humanAtTerminal{}
	bindRuntime(t, Runtime{Actor: "test", Interactive: true, Confirmer: h})
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	_, rmErr := Rm(RmParams{Path: path, Purge: true})
	if rmErr == nil {
		t.Fatal("purge succeeded though the human did not confirm")
	}
	if len(h.asked) != 1 {
		t.Fatalf("asked %d times, want 1", len(h.asked))
	}
	if h.asked[0].Verb != guard.VerbRm || h.asked[0].Path != path {
		t.Fatalf("asked about %+v", h.asked[0])
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("refused purge removed the file: %v", err)
	}

	journalPath := filepath.Join(root, ".filex", "journal", "filex.jsonl")
	data, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("refusal was not audited: %v", err)
	}
	if !strings.Contains(string(data), `"result":"denied"`) {
		t.Fatalf("journal has no denied record: %s", data)
	}
}
