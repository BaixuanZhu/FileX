//go:build integration

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests drive a real filex binary as an agent would: stdin is a pipe, so no
// human is reachable. The property being pinned down is that nothing an agent can
// put in the pipe can authorise an irreversible operation — which is only true
// because the confirmation reads a terminal and refuses to fall back to stdin.
//
// Run with: go test -tags=integration ./cmd/filex/...

type probe struct {
	t       *testing.T
	binary  string
	home    string
	work    string
	journal string
}

func newProbe(t *testing.T) *probe {
	t.Helper()

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available")
	}

	root := t.TempDir()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		resolved = root
	}

	binaryName := "filex-probe"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binary := filepath.Join(resolved, binaryName)

	build := exec.Command(goBin, "build", "-o", binary, "filex/cmd/filex")
	build.Dir = projectRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build probe: %v\n%s", err, out)
	}

	home := filepath.Join(resolved, "home")
	work := filepath.Join(resolved, "work")
	for _, dir := range []string{home, work} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	return &probe{
		t:       t,
		binary:  binary,
		home:    home,
		work:    work,
		journal: filepath.Join(home, ".filex", "journal", "filex.jsonl"),
	}
}

// projectRoot is the module directory, two levels up from cmd/filex.
func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

func (p *probe) seed(name, content string) string {
	p.t.Helper()
	path := filepath.Join(p.work, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		p.t.Fatalf("seed %s: %v", name, err)
	}
	return path
}

// run executes the probe with a pipe on stdin, the way an AI client does.
func (p *probe) run(stdin string, args ...string) (int, string) {
	p.t.Helper()
	return p.runWithEnv(nil, stdin, args...)
}

// runWithEnv adds variables on top of the probe's own environment. It replaces
// rather than appends, so a variable inherited from the developer's shell cannot
// decide the outcome of a test about that variable.
func (p *probe) runWithEnv(extra []string, stdin string, args ...string) (int, string) {
	p.t.Helper()

	cmd := exec.Command(p.binary, args...)
	cmd.Dir = p.work
	cmd.Env = append(p.env(), extra...)
	cmd.Stdin = strings.NewReader(stdin)

	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		p.t.Fatalf("run %v: %v\n%s", args, err, out)
	}
	return exitErr.ExitCode(), string(out)
}

// env points both home variables at the probe's own directory, replacing any
// inherited value rather than appending a duplicate the child may resolve either
// way.
func (p *probe) env() []string {
	out := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		switch key {
		case "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "FILEX_SESSION_ID":
			continue
		}
		out = append(out, kv)
	}
	return append(out, "HOME="+p.home, "USERPROFILE="+p.home)
}

func (p *probe) exists(name string) bool {
	p.t.Helper()
	_, err := os.Stat(filepath.Join(p.work, name))
	return err == nil
}

// config writes a real policy file into the probe's own home, the same way an
// operator would. Guard settings are only proven by an on-disk file: a unit test
// can pass a struct straight to Evaluate and never notice that the loader ignores
// a key.
func (p *probe) config(raw string) {
	p.t.Helper()
	dir := filepath.Join(p.home, ".filex")
	if err := os.MkdirAll(dir, 0755); err != nil {
		p.t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0644); err != nil {
		p.t.Fatalf("write config: %v", err)
	}
}

func (p *probe) entries() []map[string]any {
	p.t.Helper()

	data, err := os.ReadFile(p.journal)
	if err != nil {
		p.t.Fatalf("read journal: %v", err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			p.t.Fatalf("decode %q: %v", line, err)
		}
		out = append(out, e)
	}
	return out
}

// The core assertion: an agent piping "y" does not get its purge.
func TestProbePipedYesCannotAuthorisePurge(t *testing.T) {
	p := newProbe(t)
	p.seed("victim.txt", "hello")

	code, out := p.run("y\nyes\ny\n", "rm", "victim.txt", "--purge")

	if code == 0 {
		t.Fatalf("purge succeeded with a piped yes: %s", out)
	}
	if !p.exists("victim.txt") {
		t.Fatal("the file is gone")
	}
	if !strings.Contains(out, `"code":"E_DENIED"`) || !strings.Contains(out, "purge_delete") {
		t.Fatalf("want a structured denial, got: %s", out)
	}
	if !strings.Contains(out, "safer_alternative") {
		t.Fatalf("denial did not offer a way out: %s", out)
	}

	entries := p.entries()
	if len(entries) != 1 {
		t.Fatalf("want one audit record, got %d: %+v", len(entries), entries)
	}
	if entries[0]["result"] != "denied" || entries[0]["confirm_source"] != "none" {
		t.Fatalf("refusal was not audited as one: %+v", entries[0])
	}
}

// The bypass flag is the sanctioned way through, and it must leave a trace that
// says a flag — not a person — approved this.
func TestProbeBypassFlagRunsAndIsAudited(t *testing.T) {
	p := newProbe(t)
	p.seed("victim.txt", "hello")

	code, out := p.run("", "rm", "victim.txt", "--purge", bypassFlag)
	if code != 0 {
		t.Fatalf("bypass flag did not authorise the purge: exit %d, %s", code, out)
	}
	if p.exists("victim.txt") {
		t.Fatal("file survived a bypassed purge")
	}

	entries := p.entries()
	if len(entries) != 1 {
		t.Fatalf("want one audit record, got %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e["result"] != "executed" || e["bypass"] != true {
		t.Fatalf("bypass was not recorded as one: %+v", e)
	}
	if e["confirm_source"] != "bypass:flag" {
		t.Fatalf("confirm_source = %v, want bypass:flag", e["confirm_source"])
	}
	// The guard's own objection stays in the record, so the log shows what was
	// overridden rather than only what was allowed.
	if e["guard_class"] != "purge_delete" {
		t.Fatalf("guard_class = %v, want purge_delete", e["guard_class"])
	}
}

// allow_bypass=false must be stronger than the flag, even when the flag is real.
func TestProbeBypassInertWhenConfigRefuses(t *testing.T) {
	p := newProbe(t)
	p.config(`{"guard":{"allow_bypass":false}}`)
	p.seed("victim.txt", "hello")

	code, out := p.run("", "rm", "victim.txt", "--purge", bypassFlag)
	if code == 0 {
		t.Fatalf("bypass worked though allow_bypass is false: %s", out)
	}
	if !p.exists("victim.txt") {
		t.Fatal("an inert bypass still deleted the file")
	}
}

// guard.enabled=false is an operator setting, so it has to work through the file
// on disk. Unit tests hand the struct straight to Evaluate and would never notice
// a key the loader does not read.
func TestProbeGuardDisabledByConfigRunsPurge(t *testing.T) {
	p := newProbe(t)
	p.config(`{"guard":{"enabled":false}}`)
	p.seed("victim.txt", "hello")

	// No terminal, no bypass flag: the config is the only thing authorising this.
	code, out := p.run("y\n", "rm", "victim.txt", "--purge")
	if code != 0 {
		t.Fatalf("guard.enabled=false was not honoured: exit %d, %s", code, out)
	}
	if p.exists("victim.txt") {
		t.Fatal("the purge was refused despite a disabled guard")
	}

	entries := p.entries()
	if len(entries) != 1 {
		t.Fatalf("want one audit record, got %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e["result"] != "executed" || e["decision"] != "allow" {
		t.Fatalf("record is not an allowed execution: %+v", e)
	}
	// The record says the guard was off rather than staying silent about it, so
	// the log can tell "operator disabled the guard" apart from "guard never
	// applied to this verb".
	if e["guard_class"] != "disabled" {
		t.Fatalf("guard_class = %v, want disabled", e["guard_class"])
	}
	if v, ok := e["bypass"]; ok && v != false {
		t.Fatalf("a disabled guard recorded a bypass: %+v", e)
	}
}

// A config file that does not parse must not read as "guard disabled". The
// loader falls back to defaults, and the defaults keep the guard on — so the
// bypass flag here is allowed, and refusing it would be a different bug.
func TestProbeBrokenConfigDoesNotDisableGuard(t *testing.T) {
	p := newProbe(t)
	p.config(`{"guard":{"enabled":false`)
	p.seed("victim.txt", "hello")

	code, out := p.run("", "rm", "victim.txt", "--purge")
	if code == 0 {
		t.Fatalf("a truncated config file disabled the guard: %s", out)
	}
	if !p.exists("victim.txt") {
		t.Fatal("the file was deleted on a config file that does not parse")
	}
	if !strings.Contains(out, "purge_delete") {
		t.Fatalf("guard did not classify the denied purge: %s", out)
	}
}

func TestProbeRecoverableRemoveIsNotGated(t *testing.T) {
	p := newProbe(t)
	p.seed("victim.txt", "hello")

	code, out := p.run("", "rm", "victim.txt")
	if code != 0 {
		t.Fatalf("recoverable rm needs approval it should not need: exit %d, %s", code, out)
	}
	if p.exists("victim.txt") {
		t.Fatal("file survived")
	}
	if _, err := os.Stat(filepath.Join(p.home, ".filex", "trash")); err != nil {
		t.Fatalf("no trash directory: %v", err)
	}
}

// --yes used to be accepted and ignored. Retiring it must be an explicit refusal,
// not a silently dropped flag.
func TestProbeRmYesIsAUsageError(t *testing.T) {
	p := newProbe(t)
	p.seed("victim.txt", "hello")

	code, out := p.run("", "rm", "victim.txt", "--yes")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage): %s", code, out)
	}
	if !strings.Contains(out, "E_USAGE") || !strings.Contains(out, "--yes") {
		t.Fatalf("want a usage error naming --yes, got: %s", out)
	}
	if !p.exists("victim.txt") {
		t.Fatal("the retired flag still removed something")
	}
}

func TestProbeDryRunChangesNothing(t *testing.T) {
	p := newProbe(t)
	p.seed("victim.txt", "hello")

	code, out := p.run("", "rm", "victim.txt", "--purge", "--dry-run")
	if code != 0 {
		t.Fatalf("dry run failed: exit %d, %s", code, out)
	}
	if !p.exists("victim.txt") {
		t.Fatal("a dry run deleted the file")
	}
	if !strings.Contains(out, `"dry_run":true`) || !strings.Contains(out, "permanent_delete") {
		t.Fatalf("dry run did not describe itself: %s", out)
	}
	if _, err := os.Stat(p.journal); !os.IsNotExist(err) {
		t.Fatalf("a dry run wrote an audit record: %v", err)
	}
}

// An orchestrator that sets FILEX_SESSION_ID gets its operations grouped. The
// value is used exactly as given — not trimmed, lowercased or regenerated — so
// the id in the log is the id the orchestrator will look for.
func TestProbeSessionIDFromEnvIsRecorded(t *testing.T) {
	p := newProbe(t)
	p.seed("victim.txt", "hello")

	code, out := p.runWithEnv([]string{"FILEX_SESSION_ID=task-2026-10-30"}, "", "rm", "victim.txt")
	if code != 0 {
		t.Fatalf("recoverable rm failed: exit %d, %s", code, out)
	}

	entries := p.entries()
	if len(entries) != 1 {
		t.Fatalf("want one audit record, got %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e["session_id"] != "task-2026-10-30" {
		t.Fatalf("session_id = %v, want the value that was set", e["session_id"])
	}
	if e["session_source"] != "env" {
		t.Fatalf("session_source = %v, want env", e["session_source"])
	}
	if e["actor"] != "cli" {
		t.Fatalf("actor = %v, want cli", e["actor"])
	}
}

// Multiple invocations under one orchestrator id have to land in the same bucket,
// and each process must not invent its own id.
func TestProbeSameEnvIDGroupsSeparateProcesses(t *testing.T) {
	p := newProbe(t)
	for _, name := range []string{"a.txt", "b.txt"} {
		p.seed(name, "hello")
		code, out := p.runWithEnv([]string{"FILEX_SESSION_ID=same-task"}, "", "rm", name)
		if code != 0 {
			t.Fatalf("rm %s: exit %d, %s", name, code, out)
		}
	}

	entries := p.entries()
	if len(entries) != 2 {
		t.Fatalf("want two audit records, got %d: %+v", len(entries), entries)
	}
	for _, e := range entries {
		if e["session_source"] != "env" {
			t.Fatalf("record %v has source %v, want env", e["op"], e["session_source"])
		}
		if e["session_id"] != "same-task" {
			t.Fatalf("record %v has session_id %v, want the same id for both runs", e["op"], e["session_id"])
		}
	}
}

// A value that cannot safely be echoed into a JSON line is dropped rather than
// repaired, and a dropped value leaves no session fields at all — the same shape a
// run that never set the variable produces.
func TestProbeUnusableSessionIDFallsBackToUnlabelled(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"newline", "evil\nsecond-line"},
		{"tab", "evil\tsecond"},
		{"space", "with space"},
		{"quote", `with"quote`},
		{"overlong", strings.Repeat("x", 65)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newProbe(t)
			p.seed("victim.txt", "hello")

			code, out := p.runWithEnv([]string{"FILEX_SESSION_ID=" + tc.value}, "", "rm", "victim.txt")
			if code != 0 {
				t.Fatalf("rm failed: exit %d, %s", code, out)
			}
			// One record, and no session on it. A value that could break out of its
			// own field would land here as a second line — either a different record
			// or a decode failure, both caught before this assertion means anything.
			entries := p.entries()
			if len(entries) != 1 {
				t.Fatalf("want exactly one audit record, got %d: %+v", len(entries), entries)
			}
			for _, field := range []string{"session_id", "session_source"} {
				if _, ok := entries[0][field]; ok {
					t.Fatalf("%s was recorded for the unusable id %q: %+v", field, tc.value, entries[0])
				}
			}
		})
	}
}

// The aggregation runs against the same file the write path appends to, and a
// half-written tail line is a normal state of that file rather than an error. A
// read that exits non-zero would train people to ignore the audit log.
func TestProbeJournalStatsSkipsCorruptTailAndChangesNothing(t *testing.T) {
	p := newProbe(t)
	dir := filepath.Join(p.home, ".filex", "journal")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	withTail := statsFixture + `{"timestamp":"2026-10-30T09:30:00Z","op":"grep","resu`
	if err := os.WriteFile(filepath.Join(dir, "filex.jsonl"), []byte(withTail), 0644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	before := fileSize(t, filepath.Join(dir, "filex.jsonl"))

	code, out := p.run("", "journal", "stats")
	if code != 0 {
		t.Fatalf("journal stats on a log with a truncated tail exited %d: %s", code, out)
	}
	for _, want := range []string{"decision gate", "session key share", "session source", "guard classes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stats output is missing the %q section:\n%s", want, out)
		}
	}
	// 5 lines: 3 records, one corrupt line, one truncated tail.
	if !strings.Contains(out, "5 lines, 3 parsed, 2 skipped") {
		t.Fatalf("line accounting is wrong:\n%s", out)
	}

	if after := fileSize(t, filepath.Join(dir, "filex.jsonl")); after != before {
		t.Fatalf("a read-only command modified the audit log (%d bytes -> %d)", before, after)
	}

	// --json carries the same numbers under fixed field names.
	code, out = p.run("", "journal", "stats", "--json")
	if code != 0 {
		t.Fatalf("journal stats --json exited %d: %s", code, out)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Lines   int `json:"lines"`
			Skipped int `json:"skipped_lines"`
			Matched int `json:"matched_records"`
			Gate    struct {
				SessionKeyShare float64 `json:"session_key_share"`
				LabelledRecords int     `json:"labelled_records"`
			} `json:"gate"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("decode --json output %q: %v", out, err)
	}
	if !envelope.OK || envelope.Data.Lines != 5 || envelope.Data.Skipped != 2 || envelope.Data.Matched != 3 {
		t.Fatalf("json counts = %+v, want 5 lines / 2 skipped / 3 matched", envelope.Data)
	}
	if envelope.Data.Gate.LabelledRecords != 2 || envelope.Data.Gate.SessionKeyShare != 0.667 {
		t.Fatalf("gate M1 = %+v, want 2 of 3", envelope.Data.Gate)
	}
}

// A fresh install has no audit log yet. That is an answer, not a failure.
func TestProbeJournalStatsWithoutALog(t *testing.T) {
	p := newProbe(t)

	code, out := p.run("", "journal", "stats")
	if code != 0 {
		t.Fatalf("journal stats with no history exited %d: %s", code, out)
	}
	if !strings.Contains(out, "0 lines, 0 parsed") {
		t.Fatalf("no-history output is not a clean zero:\n%s", out)
	}
	journalPath := filepath.Join(p.home, ".filex", "journal")
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("reading history created %s (err %v)", journalPath, err)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}
