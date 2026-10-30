package journal_test

import (
	"os"
	"path/filepath"
	"testing"

	"filex/internal/journal"
)

// isolate points both home variables and the working directory at a temp dir, so
// a test never appends to the developer's real audit log.
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

func TestRecord(t *testing.T) {
	root := isolate(t)

	entry := journal.Entry{
		Op:           "write",
		Path:         `C:\test\file.txt`,
		SHA256Before: "abc123",
		SHA256After:  "def456",
	}

	if err := journal.Record(entry); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	journalPath := filepath.Join(root, ".filex", "journal", "filex.jsonl")
	data, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}

	if len(data) == 0 {
		t.Error("journal file is empty after Record")
	}
}

func TestIsSensitiveName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"PATH", false},
		{"MY_PASSWORD", true},
		{"API_KEY", true},
		{"SECRET_TOKEN", true},
		{"PRIVATE_KEY", true},
		{"HOME", false},
		{"CREDENTIAL_FILE", true},
	}

	for _, tt := range tests {
		got := journal.IsSensitiveName(tt.name)
		if got != tt.want {
			t.Errorf("IsSensitiveName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestHashEnvValue(t *testing.T) {
	hash1 := journal.HashEnvValue("secret")
	hash2 := journal.HashEnvValue("secret")
	hash3 := journal.HashEnvValue("different")

	if hash1 != hash2 {
		t.Error("same value should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("different values should produce different hashes")
	}
	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash1))
	}
}

func TestMaskEnvEntry(t *testing.T) {
	entry := journal.MaskEnvEntry("env_set", "user", "API_KEY", "my-secret-key")

	if entry.EnvName != "API_KEY" {
		t.Errorf("EnvName = %q, want API_KEY", entry.EnvName)
	}
	if entry.EnvValueHash == "" {
		t.Error("sensitive value should be hashed, got empty")
	}

	normalEntry := journal.MaskEnvEntry("env_set", "user", "PATH", "C:\\bin")
	if normalEntry.EnvValueHash != "" {
		t.Error("non-sensitive value should not be hashed")
	}
}
