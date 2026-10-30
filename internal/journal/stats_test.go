package journal_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filex/internal/journal"
)

// fixture mixes four things a real log has: records written before the session
// fields existed, records from the CLI with an env session, records from two
// different MCP hosts, and lines that are not readable at all.
const fixture = `{"timestamp":"2026-10-28T10:00:00Z","op":"write","path":"C:\\old\\a.txt","sha256_before":"aa","sha256_after":"bb","result":"executed"}
CORRUPT NOT JSON AT ALL
{"timestamp":"2026-10-29T10:00:00Z","op":"rm","path":"C:\\b.txt","actor":"cli","session_id":"env-task","session_source":"env","guard_class":"purge_delete","decision":"deny","confirm_source":"none","result":"denied"}
{"timestamp":"2026-10-29T10:05:00Z","op":"rm","path":"C:\\c.txt","actor":"cli","session_id":"env-task","session_source":"env","guard_class":"purge_delete","decision":"deny","confirm_source":"bypass:flag","bypass":true,"result":"executed"}
{"timestamp":"2026-10-29T10:10:00Z","op":"stat","path":"C:\\c.txt","actor":"cli","session_id":"env-task","session_source":"env","result":"executed"}
{"op":"read","actor":"mcp:cursor","session_id":"aaaa","session_source":"mcp_connection","decision":"allow","result":"executed"}
{"timestamp":"2026-10-29T11:30:00Z","op":"edit","path":"C:\\d.txt","decision":"allow","result":"executed"}
{"timestamp":"2026-10-30T09:00:00Z","op":"tail","actor":"mcp:claude-code","session_id":"bbbb","session_source":"mcp_connection","result":"executed"}
{"timestamp":"2026-10-30T09:30:00Z","op":"grep","resu`

func scanFixture(t *testing.T, text string, opts journal.StatsOptions) *journal.Report {
	t.Helper()
	report, err := journal.Scan(strings.NewReader(text), opts)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return report
}

func TestScanCountsEveryLineOnce(t *testing.T) {
	r := scanFixture(t, fixture, journal.StatsOptions{})

	if r.Lines != 9 {
		t.Fatalf("lines = %d, want 9", r.Lines)
	}
	// The invariant the overview rests on: nothing is both counted and dropped.
	if r.Parsed+r.Skipped != r.Lines {
		t.Fatalf("parsed %d + skipped %d != lines %d", r.Parsed, r.Skipped, r.Lines)
	}
	if r.Skipped != 2 {
		t.Fatalf("skipped = %d, want 2 (the corrupt line and the truncated tail)", r.Skipped)
	}
	if r.Matched != 7 || r.Filtered != 0 {
		t.Fatalf("matched %d / filtered %d, want 7 / 0", r.Matched, r.Filtered)
	}
	if r.First != "2026-10-28T10:00:00Z" || r.Last != "2026-10-30T09:00:00Z" {
		t.Fatalf("span = %q .. %q", r.First, r.Last)
	}
}

func TestScanGroupsSources(t *testing.T) {
	r := scanFixture(t, fixture, journal.StatsOptions{})

	// A record with no session_source at all — written before the field existed —
	// is absent, not a fourth category.
	want := map[string]int{"absent": 2, "env": 3, "mcp_connection": 2}
	if len(r.Sources) != len(want) {
		t.Fatalf("sources = %+v, want %d buckets", r.Sources, len(want))
	}
	for _, g := range r.Sources {
		if want[g.Key] != g.Count {
			t.Fatalf("source %q = %d, want %d (all: %+v)", g.Key, g.Count, want[g.Key], r.Sources)
		}
	}
	// Shares are over matched records and add up.
	total := 0.0
	for _, g := range r.Sources {
		total += g.Share
	}
	if total < 0.99 || total > 1.01 {
		t.Fatalf("source shares sum to %v, want about 1", total)
	}
}

func TestScanGroupsVerbs(t *testing.T) {
	r := scanFixture(t, fixture, journal.StatsOptions{})
	want := map[string]int{"write": 1, "rm": 2, "read": 1, "edit": 1, "tail": 1, "stat": 1}
	for verb, count := range want {
		if got := countOf(r.Verbs, verb); got != count {
			t.Fatalf("verb %q = %d, want %d", verb, got, count)
		}
	}
	// Highest count first, so a human reads the busiest verbs at the top.
	if len(r.Verbs) == 0 || r.Verbs[0].Key != "rm" {
		t.Fatalf("verbs are not ordered by volume: %+v", r.Verbs)
	}
}

func TestScanGuardClassesCarryTheDecisionChain(t *testing.T) {
	r := scanFixture(t, fixture, journal.StatsOptions{})
	if len(r.Classes) != 1 {
		t.Fatalf("classes = %+v, want only purge_delete", r.Classes)
	}
	c := r.Classes[0]
	if c.Class != "purge_delete" || c.Records != 2 {
		t.Fatalf("class = %+v", c)
	}
	if c.Decisions["deny"] != 2 {
		t.Fatalf("decisions = %+v, want two denies", c.Decisions)
	}
	// The bypassed purge stays in the same class: what was overridden has to be
	// visible next to what was refused.
	if c.Bypassed != 1 || c.Denied != 1 {
		t.Fatalf("bypassed %d / denied %d, want 1 / 1", c.Bypassed, c.Denied)
	}
}

func TestScanSessions(t *testing.T) {
	r := scanFixture(t, fixture, journal.StatsOptions{Top: 20})
	if r.SessionsTotal != 4 {
		t.Fatalf("sessions total = %d, want 3 labelled plus one unlabelled bucket: %+v", r.SessionsTotal, r.Sessions)
	}
	if r.Sessions[0].SessionID != "env-task" || r.Sessions[0].Operations != 3 {
		t.Fatalf("busiest session = %+v, want env-task with 3 operations", r.Sessions[0])
	}
	if r.Sessions[0].HighRisk != 2 || r.Sessions[0].Denied != 1 || r.Sessions[0].Bypassed != 1 {
		t.Fatalf("env-task totals = %+v", r.Sessions[0])
	}
	if r.SessionsOmit != 0 {
		t.Fatalf("omitted = %d with top 20", r.SessionsOmit)
	}

	small := scanFixture(t, fixture, journal.StatsOptions{Top: 1})
	if len(small.Sessions) != 1 || small.SessionsOmit != 3 || small.SessionsTotal != 4 {
		t.Fatalf("top 1 gave %d shown / %d omitted of %d", len(small.Sessions), small.SessionsOmit, small.SessionsTotal)
	}

	for _, s := range r.Sessions {
		if s.SessionID == "" {
			if s.Source != journal.SessionSourceAbsent {
				t.Fatalf("unlabelled bucket has source %q", s.Source)
			}
			continue
		}
		if s.Source == journal.SessionSourceAbsent {
			t.Fatalf("session %q carries an id but no source", s.SessionID)
		}
	}
}

func TestScanGateNumbers(t *testing.T) {
	r := scanFixture(t, fixture, journal.StatsOptions{})
	g := r.Gate

	// M1: five of seven records carry a session id.
	if g.LabelledRecords != 5 || g.SessionKeyShare != 0.714 {
		t.Fatalf("M1 = %d labelled, share %v, want 5 and 0.714", g.LabelledRecords, g.SessionKeyShare)
	}
	// M2: one session did two guarded operations, so every percentile is 2.
	if g.HighRiskSessions != 1 || g.HighRiskP50 != 2 || g.HighRiskP99 != 2 || g.HighRiskMax != 2 {
		t.Fatalf("M2 = %+v, want p50/p99/max of 2 over one session", g)
	}
	// M3: one refusal, one flag-approved override.
	if g.Denials != 1 || g.Bypasses != 1 {
		t.Fatalf("M3 = denials %d / bypasses %d, want 1 / 1", g.Denials, g.Bypasses)
	}
	// Two hosts is the minimum that makes M1 mean anything.
	if strings.Join(g.Hosts, ",") != "claude-code,cursor" {
		t.Fatalf("hosts = %v, want claude-code and cursor", g.Hosts)
	}
	if len(r.Bypass.Sessions) != 1 || r.Bypass.Sessions[0] != "env-task" {
		t.Fatalf("bypass sessions = %v, want env-task", r.Bypass.Sessions)
	}
}

// A record with no timestamp cannot be placed in a window, so no window drops it.
func TestScanTimeWindowBoundaries(t *testing.T) {
	since := time.Date(2026, 10, 29, 10, 5, 0, 0, time.UTC)
	until := time.Date(2026, 10, 29, 11, 0, 0, 0, time.UTC)
	r := scanFixture(t, fixture, journal.StatsOptions{Since: since, Until: until})

	// Both endpoints are inclusive; the untimestamped read stays in.
	if r.Matched != 3 {
		t.Fatalf("matched = %d, want 3: %+v", r.Matched, r.Verbs)
	}
	if r.Filtered != 4 {
		t.Fatalf("filtered = %d, want 4", r.Filtered)
	}
	if countOf(r.Verbs, "rm") != 1 || countOf(r.Verbs, "read") != 1 || countOf(r.Verbs, "stat") != 1 {
		t.Fatalf("window kept the wrong records: %+v", r.Verbs)
	}
	if r.First != "2026-10-29T10:05:00Z" || r.Last != "2026-10-29T10:10:00Z" {
		t.Fatalf("span = %q .. %q, want only the in-window timestamps", r.First, r.Last)
	}
}

func TestScanSessionFilter(t *testing.T) {
	r := scanFixture(t, fixture, journal.StatsOptions{Session: "env-task"})
	if r.Matched != 3 || r.Filtered != 4 {
		t.Fatalf("matched %d / filtered %d, want 3 / 4", r.Matched, r.Filtered)
	}
	if r.SessionsTotal != 1 || r.Sessions[0].SessionID != "env-task" {
		t.Fatalf("sessions = %+v", r.Sessions)
	}
	if r.Gate.SessionKeyShare != 1 {
		t.Fatalf("share = %v, want 1 for a session-filtered scan", r.Gate.SessionKeyShare)
	}
}

// An empty log is a normal answer: zeros everywhere and no division by anything.
func TestScanEmpty(t *testing.T) {
	for _, text := range []string{"", "\n", "  \n\n"} {
		r := scanFixture(t, text, journal.StatsOptions{})
		if r.Lines != 0 || r.Parsed != 0 || r.Matched != 0 {
			t.Fatalf("empty scan of %q gave %+v", text, r)
		}
		if r.Gate.SessionKeyShare != 0 || r.Gate.HighRiskMax != 0 {
			t.Fatalf("gate on an empty log = %+v, want zeros", r.Gate)
		}
		if len(r.Sources) != 0 || len(r.Sessions) != 0 {
			t.Fatalf("empty log produced groups: %+v %+v", r.Sources, r.Sessions)
		}
	}
}

// A log whose every line is unreadable is still reported, with the count that
// says so. Silence here would look like "nothing happened".
func TestScanAllCorrupt(t *testing.T) {
	r := scanFixture(t, "not json\nalso not json\n", journal.StatsOptions{})
	if r.Lines != 2 || r.Parsed != 0 || r.Skipped != 2 {
		t.Fatalf("r = %+v", r)
	}
}

// Collect is what the command calls, so the path handling has to be right: a
// missing log reads as empty and must not create the directory on the way.
func TestCollectWithoutALog(t *testing.T) {
	home := isolate(t)
	r, err := journal.Collect(journal.StatsOptions{})
	if err != nil {
		t.Fatalf("collect with no history: %v", err)
	}
	if r.Lines != 0 || r.Matched != 0 {
		t.Fatalf("report = %+v, want zeros", r)
	}
	dir := filepath.Join(home, ".filex", "journal")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("reading the log created %s (err %v)", dir, err)
	}
}

func TestCollectReadsTheRealPath(t *testing.T) {
	home := isolate(t)
	path, err := journal.Path()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if want := filepath.Join(home, ".filex", "journal", "filex.jsonl"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(fixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	r, err := journal.Collect(journal.StatsOptions{})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if r.Lines != 9 || r.Skipped != 2 || r.Matched != 7 {
		t.Fatalf("report = %+v", r)
	}
}

func countOf(groups []journal.Group, key string) int {
	for _, g := range groups {
		if g.Key == key {
			return g.Count
		}
	}
	return 0
}
