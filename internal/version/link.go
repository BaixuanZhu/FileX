package version

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CreateCurrentLink creates junction/symlink to version directory
func CreateCurrentLink(version string) error {
	versionDir, err := VersionDir(version)
	if err != nil {
		return err
	}

	currentLink, err := CurrentLink()
	if err != nil {
		return err
	}

	// Remove existing link if present
	if err := RemoveCurrentLink(); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		return createJunction(currentLink, versionDir)
	}
	return createSymlink(currentLink, versionDir)
}

// RemoveCurrentLink removes the current link
func RemoveCurrentLink() error {
	currentLink, err := CurrentLink()
	if err != nil {
		return err
	}

	// Check if link exists
	if _, err := os.Lstat(currentLink); os.IsNotExist(err) {
		return nil
	}

	if runtime.GOOS == "windows" {
		// Windows junction removal
		return os.Remove(currentLink)
	}
	return os.Remove(currentLink)
}

// GetCurrentVersion returns the version that current link points to
func GetCurrentVersion() (string, error) {
	currentLink, err := CurrentLink()
	if err != nil {
		return "", err
	}

	target, err := os.Readlink(currentLink)
	if err != nil {
		return "", fmt.Errorf("current link not found or invalid: %w", err)
	}

	// Extract version from path (e.g., "versions/v0.1.3" -> "0.1.3")
	base := filepath.Base(target)
	if len(base) > 1 && base[0] == 'v' {
		return base[1:], nil
	}
	return "", fmt.Errorf("invalid version directory name: %s", base)
}

// createJunction creates Windows directory junction
func createJunction(link, target string) error {
	// mklink /J link target
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create junction: %w, output: %s", err, output)
	}
	return nil
}

// createSymlink creates Unix symbolic link
func createSymlink(link, target string) error {
	return os.Symlink(target, link)
}

// LinkExists checks if the current link exists
func LinkExists() bool {
	currentLink, err := CurrentLink()
	if err != nil {
		return false
	}

	_, err = os.Lstat(currentLink)
	return err == nil
}

// GetInstalledVersions returns list of installed versions
func GetInstalledVersions() ([]string, error) {
	versionsDir, err := VersionsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "v") {
			versions = append(versions, entry.Name()[1:]) // Remove 'v' prefix
		}
	}

	return versions, nil
}
