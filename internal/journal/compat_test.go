package journal_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filex/internal/journal"
)

// The audit log is append-only and outlives any one version of the tool, so a
// record written today has to stay readable by whatever parses old files, and a
// record that predates the guard fields must still parse today. Every new field
// is omitempty for exactly this reason; this test is what keeps that true.
func TestLegacyRecordBytesUnchanged(t *testing.T) {
	cases := []struct {
		name  string
		entry journal.Entry
		want  string
	}{
		{
			name: "write",
			entry: journal.Entry{
				Timestamp:    "2026-01-02T03:04:05Z",
				Op:           "write",
				Path:         `C:\work\notes.txt`,
				BackupID:     "20260102-030405",
				SHA256Before: "aaa",
				SHA256After:  "bbb",
			},
			want: `{"timestamp":"2026-01-02T03:04:05Z","op":"write","path":"C:\\work\\notes.txt",` +
				`"backup_id":"20260102-030405","sha256_before":"aaa","sha256_after":"bbb"}`,
		},
		{
			name: "rm",
			entry: journal.Entry{
				Timestamp: "2026-01-02T03:04:05Z",
				Op:        "rm",
				Path:      `C:\work\gone.txt`,
			},
			want: `{"timestamp":"2026-01-02T03:04:05Z","op":"rm","path":"C:\\work\\gone.txt"}`,
		},
		{
			name: "env unset",
			entry: journal.Entry{
				Timestamp: "2026-01-02T03:04:05Z",
				Op:        "env_unset",
				Scope:     "User",
				EnvName:   "MY_TOOL",
			},
			want: `{"timestamp":"2026-01-02T03:04:05Z","op":"env_unset","scope":"User","env_name":"MY_TOOL"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := isolate(t)

			if err := journal.Record(tc.entry); err != nil {
				t.Fatalf("Record: %v", err)
			}

			data, err := os.ReadFile(filepath.Join(root, ".filex", "journal", "filex.jsonl"))
			if err != nil {
				t.Fatalf("read journal: %v", err)
			}
			line := strings.TrimSuffix(string(data), "\n")
			if line != tc.want {
				t.Fatalf("record changed shape on disk:\n got %s\nwant %s", line, tc.want)
			}
		})
	}
}

// A file written by an older build has none of the guard fields. Reading it must
// succeed and leave those fields empty rather than guessing a value — an empty
// decision means "this build never asked", which stats must be able to tell apart
// from an actual answer.
func TestDecodingLegacyLine(t *testing.T) {
	legacy := `{"timestamp":"2025-11-30T10:00:00Z","op":"edit","path":"/tmp/a.txt","sha256_before":"x","sha256_after":"y"}`

	var e journal.Entry
	if err := json.Unmarshal([]byte(legacy), &e); err != nil {
		t.Fatalf("decode legacy record: %v", err)
	}

	if e.Op != "edit" || e.Path != "/tmp/a.txt" || e.SHA256Before != "x" || e.SHA256After != "y" {
		t.Fatalf("legacy fields lost: %+v", e)
	}
	if e.Actor != "" || e.SessionID != "" || e.GuardClass != "" || e.Decision != "" || e.Result != "" {
		t.Fatalf("a legacy record must not gain invented values: %+v", e)
	}
	if e.Bypass {
		t.Fatal("a legacy record must not read as a bypass")
	}
}

func TestRecordFillsTimestampOnlyWhenMissing(t *testing.T) {
	root := isolate(t)

	if err := journal.Record(journal.Entry{Timestamp: "2026-01-02T03:04:05Z", Op: "hash", Path: "a"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := journal.Record(journal.Entry{Op: "hash", Path: "b"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".filex", "journal", "filex.jsonl"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), data)
	}
	if !strings.HasPrefix(lines[0], `{"timestamp":"2026-01-02T03:04:05Z"`) {
		t.Fatalf("a supplied timestamp was overwritten: %s", lines[0])
	}
	var second journal.Entry
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if second.Timestamp == "" {
		t.Fatal("a missing timestamp was left missing; records have to be orderable")
	}
}
