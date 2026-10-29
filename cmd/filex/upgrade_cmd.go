package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"filex/internal/buildinfo"
	"filex/internal/errs"
	"filex/internal/version"
)

const (
	githubAPIURL   = "https://api.github.com/repos/BaixuanZhu/FileX/releases/latest"
	githubOwner    = "BaixuanZhu"
	githubRepo     = "FileX"
	downloadBuffer = 32 * 1024
	maxVersions    = 3
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
	// Parse flags
	checkOnly := false
	autoYes := false
	listOnly := false
	cleanupOnly := false
	useVersion := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--check":
			checkOnly = true
		case "--yes", "-y":
			autoYes = true
		case "--list":
			listOnly = true
		case "--cleanup":
			cleanupOnly = true
		case "--use":
			if i+1 >= len(args) {
				return errs.New(errs.EUsage, "--use requires a version argument")
			}
			i++
			useVersion = args[i]
		}
	}

	// Handle --list
	if listOnly {
		return listVersions()
	}

	// Handle --cleanup
	if cleanupOnly {
		return cleanupVersions()
	}

	// Handle --use
	if useVersion != "" {
		return switchVersion(useVersion)
	}

	// Get current version
	currentVersion, err := getCurrentInstalledVersion()
	if err != nil {
		return errs.Newf(errs.EInternal, "failed to get current version: %v", err)
	}

	// Fetch latest release
	release, releaseErr := fetchLatestRelease()
	if releaseErr != nil {
		return releaseErr
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

	// Perform upgrade
	if err := performUpgrade(latestVersion, release); err != nil {
		return errs.Newf(errs.EIO, "upgrade failed: %v", err)
	}

	result.Upgraded = true
	result.Message = fmt.Sprintf("Successfully upgraded from %s to %s", currentVersion, latestVersion)

	// Automatic cleanup
	if cleanupErr := autoCleanup(); cleanupErr != nil {
		result.Message += fmt.Sprintf("\nWarning: cleanup failed: %v", cleanupErr)
	}

	result.Message += "\nNew version will be used on next invocation"
	return writeUpgradeResult(result)
}

func getCurrentInstalledVersion() (string, error) {
	// Try to get from version management system
	if version.LinkExists() {
		v, err := version.GetCurrentVersion()
		if err == nil {
			return v, nil
		}
	}

	// Fallback to buildinfo
	return buildinfo.Get().Version, nil
}

func performUpgrade(newVersion string, release *githubRelease) error {
	// Determine archive name
	archiveName := getArchiveName(newVersion)

	// Find archive asset
	var archiveAsset *githubAsset
	var checksumAsset *githubAsset
	for i := range release.Assets {
		if release.Assets[i].Name == archiveName {
			archiveAsset = &release.Assets[i]
		}
		if release.Assets[i].Name == "checksums.txt" {
			checksumAsset = &release.Assets[i]
		}
	}

	if archiveAsset == nil {
		return fmt.Errorf("archive asset '%s' not found in release", archiveName)
	}

	// Download archive to temporary directory
	tempDir, err := os.MkdirTemp("", "filex-upgrade-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, archiveName)
	if err := downloadFile(archiveAsset.BrowserDownloadURL, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Verify checksum if available
	if checksumAsset != nil {
		expectedChecksum, err := fetchExpectedChecksum(checksumAsset.BrowserDownloadURL, archiveName)
		if err == nil {
			actualChecksum, err := computeSHA256(archivePath)
			if err != nil {
				return fmt.Errorf("checksum computation failed: %w", err)
			}
			if actualChecksum != expectedChecksum {
				return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
			}
		}
	}

	// Extract archive
	extractDir := filepath.Join(tempDir, "extracted")
	if err := extractArchive(archivePath, extractDir); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Find extracted directory
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return err
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return fmt.Errorf("unexpected archive structure")
	}
	extractedDir := filepath.Join(extractDir, entries[0].Name())

	// Create version directory
	versionDir, err := version.VersionDir(newVersion)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return err
	}

	// Move binary from extracted archive
	binaryName := "filex"
	if runtime.GOOS == "windows" {
		binaryName = "filex.exe"
	}
	srcBinary := filepath.Join(extractedDir, binaryName)
	dstBinary := filepath.Join(versionDir, binaryName)
	if err := moveFile(srcBinary, dstBinary); err != nil {
		os.RemoveAll(versionDir)
		return err
	}

	// Copy docs if present
	srcDocs := filepath.Join(extractedDir, "docs")
	if _, err := os.Stat(srcDocs); err == nil {
		dstDocs := filepath.Join(versionDir, "docs")
		if err := copyDir(srcDocs, dstDocs); err != nil {
			fmt.Printf("Warning: failed to copy docs: %v\n", err)
		}
	}

	// Copy README and LICENSE if present
	for _, file := range []string{"README.md", "LICENSE"} {
		src := filepath.Join(extractedDir, file)
		dst := filepath.Join(versionDir, file)
		if _, err := os.Stat(src); err == nil {
			if err := copyFile(src, dst); err != nil {
				fmt.Printf("Warning: failed to copy %s: %v\n", file, err)
			}
		}
	}

	// Switch current link
	if err := version.CreateCurrentLink(newVersion); err != nil {
		os.RemoveAll(versionDir)
		return err
	}

	return nil
}

func getArchiveName(ver string) string {
	platform := runtime.GOOS
	arch := runtime.GOARCH

	var osName string
	switch platform {
	case "windows":
		osName = "windows"
	case "linux":
		osName = "linux"
	case "darwin":
		osName = "darwin"
	default:
		osName = platform
	}

	var ext string
	if platform == "windows" {
		ext = "zip"
	} else {
		ext = "tar.gz"
	}

	return fmt.Sprintf("filex-v%s-%s-%s.%s", ver, osName, arch, ext)
}

func extractArchive(archivePath, destDir string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, destDir)
	} else if strings.HasSuffix(archivePath, ".tar.gz") {
		return extractTarGz(archivePath, destDir)
	}
	return fmt.Errorf("unsupported archive format")
}

func extractZip(zipPath, destDir string) error {
	if runtime.GOOS == "windows" {
		// Use PowerShell on Windows
		cmd := fmt.Sprintf("powershell -Command \"Expand-Archive -Path '%s' -DestinationPath '%s'\"", zipPath, destDir)
		output, err := exec.Command("cmd", "/c", cmd).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to extract zip: %w, output: %s", err, output)
		}
		return nil
	}
	// Unix: use archive/zip package
	return extractZipNative(zipPath, destDir)
}

func extractZipNative(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)

		// Check for ZipSlip vulnerability
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(tarGzPath, destDir string) error {
	file, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		// Check for path traversal
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", target)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), os.ModePerm); err != nil {
				return err
			}
			outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}

func moveFile(src, dst string) error {
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func autoCleanup() error {
	versionsDir, err := version.VersionsDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Collect version directories
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "v") {
			versions = append(versions, entry.Name())
		}
	}

	// If <= maxVersions, nothing to clean
	if len(versions) <= maxVersions {
		return nil
	}

	// Sort by version (descending, newest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] > versions[j]
	})

	// Get current version to avoid removing it
	currentVersion, _ := version.GetCurrentVersion()

	// Remove versions beyond the last maxVersions
	removedCount := 0
	for _, v := range versions[maxVersions:] {
		versionName := strings.TrimPrefix(v, "v")
		if v == "v"+currentVersion {
			continue // Never remove current version
		}
		versionDir := filepath.Join(versionsDir, v)
		if err := os.RemoveAll(versionDir); err != nil {
			fmt.Printf("Warning: failed to remove %s: %v\n", versionName, err)
		} else {
			removedCount++
		}
	}

	if removedCount > 0 {
		fmt.Printf("Cleaned up %d old version(s)\n", removedCount)
	}

	return nil
}

func listVersions() *errs.Error {
	versions, err := version.GetInstalledVersions()
	if err != nil {
		return errs.Newf(errs.EInternal, "failed to list versions: %v", err)
	}

	if len(versions) == 0 {
		fmt.Println("No versions installed")
		return nil
	}

	currentVersion, _ := version.GetCurrentVersion()

	result := map[string]interface{}{
		"versions":        versions,
		"current_version": currentVersion,
	}

	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "failed to write result: %v", err)
	}
	return nil
}

func switchVersion(targetVersion string) *errs.Error {
	// Verify version exists
	versions, err := version.GetInstalledVersions()
	if err != nil {
		return errs.Newf(errs.EInternal, "failed to get installed versions: %v", err)
	}

	found := false
	for _, v := range versions {
		if v == targetVersion {
			found = true
			break
		}
	}

	if !found {
		return errs.Newf(errs.ENotFound, "version %s not installed", targetVersion)
	}

	if err := version.CreateCurrentLink(targetVersion); err != nil {
		return errs.Newf(errs.EInternal, "failed to switch version: %v", err)
	}

	result := map[string]string{
		"message": fmt.Sprintf("Switched to v%s", targetVersion),
		"note":    "New version will be used on next invocation",
	}
	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "failed to write result: %v", err)
	}
	return nil
}

func cleanupVersions() *errs.Error {
	versionsDir, err := version.VersionsDir()
	if err != nil {
		return errs.Newf(errs.EInternal, "failed to get versions directory: %v", err)
	}

	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return errs.Newf(errs.EInternal, "failed to read versions directory: %v", err)
	}

	// Collect version directories
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "v") {
			versions = append(versions, entry.Name())
		}
	}

	// Sort by version (newest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] > versions[j]
	})

	// Keep last maxVersions, remove rest
	if len(versions) <= maxVersions {
		result := map[string]string{
			"message": fmt.Sprintf("Nothing to clean up (%d version(s) installed, keeping %d)", len(versions), maxVersions),
		}
		if err := errs.WriteSuccess(os.Stdout, result); err != nil {
			return errs.Newf(errs.EIO, "failed to write result: %v", err)
		}
		return nil
	}

	currentVersion, _ := version.GetCurrentVersion()

	removedCount := 0
	for _, v := range versions[maxVersions:] {
		if v == "v"+currentVersion {
			continue // Don't remove current version
		}
		versionDir := filepath.Join(versionsDir, v)
		if err := os.RemoveAll(versionDir); err != nil {
			fmt.Printf("Warning: failed to remove %s: %v\n", v, err)
		} else {
			removedCount++
			fmt.Printf("Removed %s\n", strings.TrimPrefix(v, "v"))
		}
	}

	result := map[string]string{
		"message": fmt.Sprintf("Cleaned up %d old version(s)", removedCount),
	}
	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "failed to write result: %v", err)
	}
	return nil
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

func fetchExpectedChecksum(url, targetFile string) (string, error) {
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
	for _, line := range strings.Split(string(body), "\r\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == targetFile {
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("checksum for %s not found", targetFile)
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
