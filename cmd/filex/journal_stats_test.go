package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"filex/internal/errs"
	"filex/internal/journal"
)

// statsFixture keeps the shapes the renderer has to cope with: a record from
// before the session fields existed, a line that is not JSON at all, a
// flag-approved purge, and a long MCP session id.
const statsFixture = `{"timestamp":"2026-10-28T10:00:00Z","op":"write","path":"C:\\a.txt"}
broken line
{"timestamp":"2026-10-29T10:00:00Z","op":"rm","path":"C:\\b.txt","actor":"cli","session_id":"env-task-2026","session_source":"env","guard_class":"purge_delete","decision":"deny","confirm_source":"bypass:flag","bypass":true,"result":"executed"}
{"timestamp":"2026-10-29T11:00:00Z","op":"read","actor":"mcp:cursor","session_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","session_source":"mcp_connection","result":"executed"}
`

func scanStatsFixture(t *testing.T, opts journal.StatsOptions) *journal.Report {
	t.Helper()
	report, err := journal.Scan(strings.NewReader(statsFixture), opts)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return report
}

func TestParseBoundCountsDurationsBackwards(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

	// --since=48h means "48 hours ago", not "48 hours from now".
	got, err := parseBound("48h", now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if want := now.Add(-48 * time.Hour); !got.Equal(want) {
		t.Fatalf("48h = %v, want %v", got, want)
	}

	for _, value := range []string{"2026-10-01", "2026-10-01T08:30:00Z", "2026-10-01T08:30:00.123Z"} {
		got, err := parseBound(value, now)
		if err != nil {
			t.Fatalf("%q: %v", value, err)
		}
		if got.IsZero() {
			t.Fatalf("%q parsed to the zero time", value)
		}
	}

	for _, bad := range []string{"", "last tuesday", "48", "2026-13-01"} {
		if _, err := parseBound(bad, now); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

func TestWriteStatsRendersEverySection(t *testing.T) {
	var buf bytes.Buffer
	report := scanStatsFixture(t, journal.StatsOptions{})
	if err := writeStats(&buf, report, ""); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"4 lines, 3 parsed, 1 skipped, 3 matched",
		"decision gate",
		"session key share",
		"0.667",
		"session source",
		"guard classes",
		"purge_delete",
		"records approved by flag",
		"mcp hosts seen",
		"cursor",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output is missing %q:\n%s", want, out)
		}
	}
	// The unlabelled bucket says so instead of printing an empty cell.
	if !strings.Contains(out, "(no session)") {
		t.Fatalf("no unlabelled bucket:\n%s", out)
	}
	// A 32-character id is shortened so the table stays readable.
	if !strings.Contains(out, "bbbbbbbbbbbb…") {
		t.Fatalf("session id was not shortened:\n%s", out)
	}
}

func TestWriteStatsGroupBySelectsOneSection(t *testing.T) {
	report := scanStatsFixture(t, journal.StatsOptions{})
	for groupBy, present := range map[string]string{
		"source":  "session source",
		"verb":    "verbs",
		"class":   "guard classes",
		"session": "sessions of",
	} {
		var buf bytes.Buffer
		if err := writeStats(&buf, report, groupBy); err != nil {
			t.Fatalf("%s: %v", groupBy, err)
		}
		out := buf.String()
		if !strings.Contains(out, present) {
			t.Fatalf("--group-by=%s did not print %q:\n%s", groupBy, present, out)
		}
		// The overview always stays, so a filtered view is still traceable.
		if !strings.Contains(out, "4 lines, 3 parsed") {
			t.Fatalf("--group-by=%s dropped the overview:\n%s", groupBy, out)
		}
		if strings.Contains(out, "decision gate") {
			t.Fatalf("--group-by=%s printed another section:\n%s", groupBy, out)
		}
	}
}

func TestJournalStatsUsageErrors(t *testing.T) {
	for _, line := range []string{
		"",
		"stats --since",
		"stats --since=nonsense",
		"stats --top=0",
		"stats --top=abc",
		"stats --group-by=path",
		"stats --bogus",
		"trend",
	} {
		err := runJournal(strings.Fields(line))
		if err == nil {
			t.Fatalf("filex journal %q was accepted", line)
		}
		if err.Code != errs.EUsage {
			t.Fatalf("filex journal %q: code = %v, want EUsage", line, err.Code)
		}
	}
}

// --json is the machine-readable face of the command: fixed field names inside
// the usual envelope, so a script can read the decision-gate numbers directly.
func TestJournalStatsJSONShapeIsStable(t *testing.T) {
	var buf bytes.Buffer
	if err := errs.WriteSuccess(&buf, scanStatsFixture(t, journal.StatsOptions{})); err != nil {
		t.Fatalf("write: %v", err)
	}

	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %q: %v", buf.String(), err)
	}
	if !envelope.OK {
		t.Fatalf("envelope is not ok: %s", buf.String())
	}
	for _, field := range []string{
		"lines", "parsed_lines", "skipped_lines", "filtered_records", "matched_records",
		"sources", "verbs", "classes", "sessions", "sessions_total", "sessions_omitted",
		"bypass", "gate",
	} {
		if _, ok := envelope.Data[field]; !ok {
			t.Fatalf("json is missing %q: %s", field, buf.String())
		}
	}
	for _, field := range []string{
		"session_key_share", "labelled_records", "distinct_sessions",
		"high_risk_p50", "high_risk_p99", "high_risk_max", "denials", "bypasses", "hosts",
	} {
		gate, _ := envelope.Data["gate"].(map[string]any)
		if gate == nil {
			t.Fatalf("gate is not an object: %s", buf.String())
		}
		if _, ok := gate[field]; !ok {
			t.Fatalf("gate is missing %q: %s", field, buf.String())
		}
	}
}

// An empty list marshals as [], never null: two spellings of the same answer is
// a detail every consumer would have to remember.
func TestJournalStatsEmptyListsAreArrays(t *testing.T) {
	var buf bytes.Buffer
	empty, err := journal.Scan(strings.NewReader(""), journal.StatsOptions{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if err := errs.WriteSuccess(&buf, empty); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`"sources":[]`, `"verbs":[]`, `"classes":[]`, `"sessions":[]`, `"hosts":[]`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s in %s", want, out)
		}
	}
}
