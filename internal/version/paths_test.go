package version

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileXHome(t *testing.T) {
	home, err := FileXHome()
	if err != nil {
		t.Fatalf("FileXHome() error = %v", err)
	}

	if home == "" {
		t.Error("FileXHome() returned empty string")
	}

	if runtime.GOOS == "windows" {
		if !strings.Contains(home, "FileX") {
			t.Errorf("FileXHome() on Windows should contain 'FileX', got %s", home)
		}
	} else {
		if !strings.HasSuffix(home, ".filex") {
			t.Errorf("FileXHome() on Unix should end with '.filex', got %s", home)
		}
	}
}

func TestVersionsDir(t *testing.T) {
	versionsDir, err := VersionsDir()
	if err != nil {
		t.Fatalf("VersionsDir() error = %v", err)
	}

	home, _ := FileXHome()
	expected := filepath.Join(home, "versions")
	if versionsDir != expected {
		t.Errorf("VersionsDir() = %s, want %s", versionsDir, expected)
	}
}

func TestCurrentLink(t *testing.T) {
	currentLink, err := CurrentLink()
	if err != nil {
		t.Fatalf("CurrentLink() error = %v", err)
	}

	home, _ := FileXHome()
	expected := filepath.Join(home, "current")
	if currentLink != expected {
		t.Errorf("CurrentLink() = %s, want %s", currentLink, expected)
	}
}

func TestVersionDir(t *testing.T) {
	tests := []struct {
		version    string
		wantSuffix string
	}{
		{"0.1.3", "v0.1.3"},
		{"1.0.0", "v1.0.0"},
		{"2.3.4-beta", "v2.3.4-beta"},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			versionDir, err := VersionDir(tt.version)
			if err != nil {
				t.Fatalf("VersionDir(%s) error = %v", tt.version, err)
			}

			if !strings.HasSuffix(versionDir, tt.wantSuffix) {
				t.Errorf("VersionDir(%s) = %s, want suffix %s", tt.version, versionDir, tt.wantSuffix)
			}
		})
	}
}

func TestCurrentBinary(t *testing.T) {
	binary, err := CurrentBinary()
	if err != nil {
		t.Fatalf("CurrentBinary() error = %v", err)
	}

	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(binary, "filex.exe") {
			t.Errorf("CurrentBinary() on Windows should end with 'filex.exe', got %s", binary)
		}
	} else {
		if !strings.HasSuffix(binary, "filex") {
			t.Errorf("CurrentBinary() on Unix should end with 'filex', got %s", binary)
		}
	}
}

func TestFileXHomeWithLocalAppData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}

	// Save original value
	original := os.Getenv("LOCALAPPDATA")
	defer os.Setenv("LOCALAPPDATA", original)

	// Test with LOCALAPPDATA set
	os.Setenv("LOCALAPPDATA", "C:\\TestPath")
	home, err := FileXHome()
	if err != nil {
		t.Fatalf("FileXHome() with LOCALAPPDATA error = %v", err)
	}
	if home != "C:\\TestPath\\FileX" {
		t.Errorf("FileXHome() = %s, want C:\\TestPath\\FileX", home)
	}
}
