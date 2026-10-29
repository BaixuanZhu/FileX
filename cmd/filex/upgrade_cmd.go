package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filex/internal/buildinfo"
	"filex/internal/errs"
)

const (
	githubAPIURL   = "https://api.github.com/repos/BaixuanZhu/FileX/releases/latest"
	githubOwner    = "BaixuanZhu"
	githubRepo     = "FileX"
	binaryName     = "filex.exe"
	downloadBuffer = 32 * 1024
)

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Assets  []githubAsset `json:"assets"`
	Body    string        `json:"body"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type upgradeResult struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	UpToDate       bool   `json:"up_to_date"`
	Upgraded       bool   `json:"upgraded,omitempty"`
	Message        string `json:"message"`
}

func runUpgrade(args []string) *errs.Error {
	checkOnly := false
	autoYes := false

	for _, arg := range args {
		switch arg {
		case "--check":
			checkOnly = true
		case "--yes", "-y":
			autoYes = true
		}
	}

	currentVersion := buildinfo.Get().Version
	if currentVersion == "dev" {
		return errs.New(errs.EUnsupported, "cannot upgrade dev build; install from GitHub Releases")
	}

	// Fetch latest release
	release, err := fetchLatestRelease()
	if err != nil {
		return err
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	result := upgradeResult{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		UpToDate:       currentVersion == latestVersion,
	}

	if result.UpToDate {
		result.Message = fmt.Sprintf("Already up-to-date (version %s)", currentVersion)
		return writeUpgradeResult(result)
	}

	result.Message = fmt.Sprintf("New version available: %s → %s", currentVersion, latestVersion)

	if checkOnly {
		return writeUpgradeResult(result)
	}

	// Prompt for confirmation
	if !autoYes {
		fmt.Fprintf(os.Stderr, "Upgrade FileX from %s to %s? [y/N] ", currentVersion, latestVersion)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			result.Message = "Upgrade cancelled"
			return writeUpgradeResult(result)
		}
	}

	// Find binary asset
	var binaryAsset *githubAsset
	var checksumAsset *githubAsset
	for i := range release.Assets {
		if release.Assets[i].Name == binaryName {
			binaryAsset = &release.Assets[i]
		}
		if release.Assets[i].Name == "checksums.txt" {
			checksumAsset = &release.Assets[i]
		}
	}

	if binaryAsset == nil {
		return errs.Newf(errs.ENotFound, "binary asset '%s' not found in release", binaryName)
	}

	// Download new binary
	tempDir := os.TempDir()
	newBinaryPath := filepath.Join(tempDir, binaryName)

	if err := downloadFile(binaryAsset.BrowserDownloadURL, newBinaryPath); err != nil {
		return errs.Newf(errs.EIO, "download failed: %v", err)
	}
	defer os.Remove(newBinaryPath)

	// Verify checksum if available
	if checksumAsset != nil {
		expectedChecksum, err := fetchExpectedChecksum(checksumAsset.BrowserDownloadURL)
		if err == nil {
			actualChecksum, err := computeSHA256(newBinaryPath)
			if err != nil {
				return errs.Newf(errs.EIO, "checksum computation failed: %v", err)
			}
			if actualChecksum != expectedChecksum {
				return errs.Newf(errs.EInternal, "checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
			}
		}
	}

	// Replace current executable
	if err := replaceExecutable(newBinaryPath); err != nil {
		return errs.Newf(errs.EIO, "replacement failed: %v", err)
	}

	result.Upgraded = true
	result.Message = fmt.Sprintf("Successfully upgraded from %s to %s", currentVersion, latestVersion)
	return writeUpgradeResult(result)
}

func fetchLatestRelease() (*githubRelease, *errs.Error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", githubAPIURL, nil)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "failed to create request: %v", err)
	}
	req.Header.Set("User-Agent", "FileX-Upgrade")

	resp, err := client.Do(req)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "failed to fetch latest release: %v (check your internet connection)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errs.Newf(errs.EIO, "GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, errs.Newf(errs.EIO, "failed to parse release info: %v", err)
	}

	return &release, nil
}

func downloadFile(url, outputPath string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func fetchExpectedChecksum(url string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Parse checksums.txt format: "<hash>  <filename>"
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == binaryName {
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("checksum for %s not found", binaryName)
}

func computeSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func replaceExecutable(newBinaryPath string) error {
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable path: %w", err)
	}

	currentExe, err = filepath.Abs(currentExe)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	backupPath := currentExe + ".old"

	// Step 1: Rename current to .old
	if err := os.Rename(currentExe, backupPath); err != nil {
		return fmt.Errorf("failed to backup current executable: %w", err)
	}

	// Step 2: Copy new binary to current location
	if err := copyFile(newBinaryPath, currentExe); err != nil {
		// Rollback: restore backup
		_ = os.Rename(backupPath, currentExe)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Step 3: Delete backup on success
	_ = os.Remove(backupPath)

	return nil
}

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

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Preserve file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, sourceInfo.Mode())
}

func writeUpgradeResult(result upgradeResult) *errs.Error {
	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "failed to write result: %v", err)
	}
	return nil
}
