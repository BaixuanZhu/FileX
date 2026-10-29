package version

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCreateCurrentLink(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "filex-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override FileXHome for testing
	originalHome := os.Getenv("LOCALAPPDATA")
	if runtime.GOOS == "windows" {
		os.Setenv("LOCALAPPDATA", tempDir)
		defer os.Setenv("LOCALAPPDATA", originalHome)
	}

	// Create version directory
	versionDir := filepath.Join(tempDir, "FileX", "versions", "v0.1.3")
	if runtime.GOOS != "windows" {
		versionDir = filepath.Join(tempDir, ".filex", "versions", "v0.1.3")
	}
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatalf("Failed to create version dir: %v", err)
	}

	// Test creating link
	if err := CreateCurrentLink("0.1.3"); err != nil {
		t.Errorf("CreateCurrentLink() error = %v", err)
	}

	// Verify link exists
	currentLink := filepath.Join(tempDir, "FileX", "current")
	if runtime.GOOS != "windows" {
		currentLink = filepath.Join(tempDir, ".filex", "current")
	}
	if _, err := os.Lstat(currentLink); err != nil {
		t.Errorf("Current link not created: %v", err)
	}
}

func TestRemoveCurrentLink(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "filex-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override FileXHome for testing
	originalHome := os.Getenv("LOCALAPPDATA")
	if runtime.GOOS == "windows" {
		os.Setenv("LOCALAPPDATA", tempDir)
		defer os.Setenv("LOCALAPPDATA", originalHome)
	}

	// Create version directory and link
	versionDir := filepath.Join(tempDir, "FileX", "versions", "v0.1.3")
	if runtime.GOOS != "windows" {
		versionDir = filepath.Join(tempDir, ".filex", "versions", "v0.1.3")
	}
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatalf("Failed to create version dir: %v", err)
	}

	if err := CreateCurrentLink("0.1.3"); err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	// Test removing link
	if err := RemoveCurrentLink(); err != nil {
		t.Errorf("RemoveCurrentLink() error = %v", err)
	}

	// Verify link removed
	currentLink := filepath.Join(tempDir, "FileX", "current")
	if runtime.GOOS != "windows" {
		currentLink = filepath.Join(tempDir, ".filex", "current")
	}
	if _, err := os.Lstat(currentLink); !os.IsNotExist(err) {
		t.Errorf("Current link not removed")
	}
}

func TestGetCurrentVersion(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "filex-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override FileXHome for testing
	originalHome := os.Getenv("LOCALAPPDATA")
	if runtime.GOOS == "windows" {
		os.Setenv("LOCALAPPDATA", tempDir)
		defer os.Setenv("LOCALAPPDATA", originalHome)
	}

	// Create version directory and link
	versionDir := filepath.Join(tempDir, "FileX", "versions", "v0.1.3")
	if runtime.GOOS != "windows" {
		versionDir = filepath.Join(tempDir, ".filex", "versions", "v0.1.3")
	}
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatalf("Failed to create version dir: %v", err)
	}

	if err := CreateCurrentLink("0.1.3"); err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	// Test getting current version
	version, err := GetCurrentVersion()
	if err != nil {
		t.Errorf("GetCurrentVersion() error = %v", err)
	}
	if version != "0.1.3" {
		t.Errorf("GetCurrentVersion() = %s, want 0.1.3", version)
	}
}

func TestLinkExists(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "filex-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override FileXHome for testing
	originalHome := os.Getenv("LOCALAPPDATA")
	if runtime.GOOS == "windows" {
		os.Setenv("LOCALAPPDATA", tempDir)
		defer os.Setenv("LOCALAPPDATA", originalHome)
	}

	// Test when link doesn't exist
	if LinkExists() {
		t.Error("LinkExists() = true, want false")
	}

	// Create version directory and link
	versionDir := filepath.Join(tempDir, "FileX", "versions", "v0.1.3")
	if runtime.GOOS != "windows" {
		versionDir = filepath.Join(tempDir, ".filex", "versions", "v0.1.3")
	}
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatalf("Failed to create version dir: %v", err)
	}

	if err := CreateCurrentLink("0.1.3"); err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	// Test when link exists
	if !LinkExists() {
		t.Error("LinkExists() = false, want true")
	}
}

func TestGetInstalledVersions(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "filex-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override FileXHome for testing
	originalHome := os.Getenv("LOCALAPPDATA")
	if runtime.GOOS == "windows" {
		os.Setenv("LOCALAPPDATA", tempDir)
		defer os.Setenv("LOCALAPPDATA", originalHome)
	}

	// Create multiple version directories
	versions := []string{"v0.1.1", "v0.1.2", "v0.1.3"}
	for _, v := range versions {
		versionDir := filepath.Join(tempDir, "FileX", "versions", v)
		if runtime.GOOS != "windows" {
			versionDir = filepath.Join(tempDir, ".filex", "versions", v)
		}
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			t.Fatalf("Failed to create version dir: %v", err)
		}
	}

	// Test getting installed versions
	installed, err := GetInstalledVersions()
	if err != nil {
		t.Errorf("GetInstalledVersions() error = %v", err)
	}

	if len(installed) != 3 {
		t.Errorf("GetInstalledVersions() returned %d versions, want 3", len(installed))
	}

	// Verify versions are returned without 'v' prefix
	expected := map[string]bool{"0.1.1": true, "0.1.2": true, "0.1.3": true}
	for _, v := range installed {
		if !expected[v] {
			t.Errorf("GetInstalledVersions() returned unexpected version: %s", v)
		}
	}
}
