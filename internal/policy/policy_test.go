package policy_test

import (
	"os"
	"path/filepath"
	"testing"

	"filex/internal/policy"
)

func TestCheckCWDAllowed(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	cfg := policy.DefaultConfig()
	result := policy.Check(cwd, cfg)
	if !result.Allowed {
		t.Errorf("CWD %q should be allowed, got denied: %s", cwd, result.Reason)
	}
}

func TestCheckFileUnderCWD(t *testing.T) {
	cwd, _ := os.Getwd()
	testFile := filepath.Join(cwd, "some_file.txt")

	cfg := policy.DefaultConfig()
	result := policy.Check(testFile, cfg)
	if !result.Allowed {
		t.Errorf("file under CWD should be allowed, got denied: %s", result.Reason)
	}
}

func TestCheckWindowsDenied(t *testing.T) {
	cfg := policy.DefaultConfig()
	result := policy.Check(`C:\Windows\System32\config`, cfg)
	if result.Allowed {
		t.Error("C:\\Windows path should be denied")
	}
}

func TestCheckProgramFilesDenied(t *testing.T) {
	cfg := policy.DefaultConfig()
	result := policy.Check(`C:\Program Files\app\file.txt`, cfg)
	if result.Allowed {
		t.Error("C:\\Program Files path should be denied")
	}
}

func TestCheckGitObjectsDenied(t *testing.T) {
	cfg := policy.DefaultConfig()
	result := policy.Check(`C:\some\repo\.git\objects\ab\cdef`, cfg)
	if result.Allowed {
		t.Error(".git\\objects path should be denied")
	}
}

func TestCheckAllowedDirs(t *testing.T) {
	cfg := policy.Config{
		AllowedDirs:      []string{`D:\work`},
		BackupBeforeWrite: true,
	}

	result := policy.Check(`D:\work\project\file.txt`, cfg)
	if !result.Allowed {
		t.Errorf("path under allowed dir should be allowed, got denied: %s", result.Reason)
	}
}

func TestCheckOutsideAllowedDenied(t *testing.T) {
	cfg := policy.Config{
		AllowedDirs:      []string{`D:\work`},
		BackupBeforeWrite: true,
	}

	result := policy.Check(`E:\other\file.txt`, cfg)
	if result.Allowed {
		t.Error("path outside allowed dirs should be denied")
	}
}

func TestMatchPattern(t *testing.T) {
	cfg := policy.Config{
		AllowedDirs:  []string{`C:\`},
		DenyPatterns: []string{`Windows\**`},
	}

	result := policy.Check(`C:\Windows\System32`, cfg)
	if result.Allowed {
		t.Error("C:\\Windows\\System32 should be denied by pattern Windows\\**")
	}

	result = policy.Check(`C:\Users\test`, cfg)
	if !result.Allowed {
		t.Errorf("C:\\Users\\test should be allowed (not matching Windows\\**), got denied: %s", result.Reason)
	}

	cfg2 := policy.Config{
		AllowedDirs:  []string{`C:\`},
		DenyPatterns: []string{`.git\objects\**`},
	}

	result = policy.Check(`C:\repo\.git\objects\ab`, cfg2)
	if result.Allowed {
		t.Error(".git\\objects path should be denied")
	}

	result = policy.Check(`C:\repo\.git\config`, cfg2)
	if !result.Allowed {
		t.Errorf(".git\\config should be allowed (not .git\\objects), got denied: %s", result.Reason)
	}
}
