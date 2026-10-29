package version

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	versionsDir = "versions"
	currentLink = "current"
)

// FileXHome returns ~/.filex/ (or %LOCALAPPDATA%\FileX on Windows)
func FileXHome() (string, error) {
	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "FileX"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".filex"), nil
}

// VersionsDir returns ~/.filex/versions/
func VersionsDir() (string, error) {
	home, err := FileXHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, versionsDir), nil
}

// CurrentLink returns ~/.filex/current
func CurrentLink() (string, error) {
	home, err := FileXHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, currentLink), nil
}

// VersionDir returns ~/.filex/versions/v{version}/
func VersionDir(version string) (string, error) {
	versionsDir, err := VersionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(versionsDir, "v"+version), nil
}

// CurrentBinary returns path to current active binary
func CurrentBinary() (string, error) {
	currentLink, err := CurrentLink()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "windows" {
		return filepath.Join(currentLink, "filex.exe"), nil
	}
	return filepath.Join(currentLink, "filex"), nil
}
