package version

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// NeedsMigration checks if user has old flat installation that needs migration
func NeedsMigration() (bool, string, error) {
	// Check if already using versioned structure
	if LinkExists() {
		return false, "", nil // Already migrated
	}

	// Check for old flat installation
	exeName := "filex"
	if runtime.GOOS == "windows" {
		exeName = "filex.exe"
	}

	// Check common installation locations
	searchPaths := []string{}

	if runtime.GOOS == "windows" {
		// Check %LOCALAPPDATA%\FileX\filex.exe
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			searchPaths = append(searchPaths, filepath.Join(localAppData, "FileX", exeName))
		}
	} else {
		// Check ~/.local/bin/filex
		home, _ := os.UserHomeDir()
		searchPaths = append(searchPaths, filepath.Join(home, ".local", "bin", exeName))
		searchPaths = append(searchPaths, filepath.Join(home, ".filex", exeName))
	}

	// Check PATH locations
	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range pathDirs {
		searchPaths = append(searchPaths, filepath.Join(dir, exeName))
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			// Found old installation
			return true, path, nil
		}
	}

	return false, "", nil
}

// MigrateFromFlat migrates from flat installation to versioned structure
func MigrateFromFlat(oldBinaryPath string) error {
	// Get version from old binary by running it
	currentVersion, err := getBinaryVersion(oldBinaryPath)
	if err != nil {
		// If we can't get version, use "unknown"
		currentVersion = "unknown"
	}

	// Create version directory
	versionDir, err := VersionDir(currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get version directory: %w", err)
	}

	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return fmt.Errorf("failed to create version directory: %w", err)
	}

	// Copy old binary to versioned directory
	binaryName := "filex"
	if runtime.GOOS == "windows" {
		binaryName = "filex.exe"
	}

	newBinaryPath := filepath.Join(versionDir, binaryName)
	if err := copyFile(oldBinaryPath, newBinaryPath); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Make binary executable on Unix
	if runtime.GOOS != "windows" {
		if err := os.Chmod(newBinaryPath, 0755); err != nil {
			return fmt.Errorf("failed to set executable permission: %w", err)
		}
	}

	// Create current link
	if err := CreateCurrentLink(currentVersion); err != nil {
		return fmt.Errorf("failed to create current link: %w", err)
	}

	return nil
}

// getBinaryVersion runs the binary with "version" command and extracts version
func getBinaryVersion(binaryPath string) (string, error) {
	// This is a simplified version - in practice, you'd need to execute the binary
	// and parse its output. For now, we'll return an error to use "unknown"
	return "", fmt.Errorf("version detection not implemented")
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := destFile.ReadFrom(sourceFile); err != nil {
		return err
	}

	// Preserve file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, sourceInfo.Mode())
}
