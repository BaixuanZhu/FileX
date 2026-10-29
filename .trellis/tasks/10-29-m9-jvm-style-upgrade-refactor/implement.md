# M9 Implementation Plan

## Phase 1: Core Infrastructure (2-3 hours)

### 1.1 Define Directory Structure Constants

**File**: `internal/version/paths.go` (new)

```go
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
```

**Tests**: `internal/version/paths_test.go`
- Test FileXHome() on Windows/Unix
- Test VersionsDir()
- Test CurrentLink()
- Test VersionDir() with various versions

### 1.2 Implement Link Management

**File**: `internal/version/link.go` (new)

```go
package version

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
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
```

**Tests**: `internal/version/link_test.go`
- Test CreateCurrentLink() on Windows/Unix
- Test RemoveCurrentLink()
- Test GetCurrentVersion()
- Test junction creation on Windows
- Test symlink creation on Unix

### 1.3 Create Wrapper Scripts

**Windows Wrapper**: `scripts/filex.bat` (new)

```batch
@echo off
setlocal

set FILEX_HOME=%LOCALAPPDATA%\FileX
set FILEX_CURRENT=%FILEX_HOME%\current

if not exist "%FILEX_CURRENT%\filex.exe" (
    echo Error: FileX current version not found
    echo Please run: filex upgrade
    exit /b 1
)

"%FILEX_CURRENT%\filex.exe" %*
```

**Unix Wrapper**: `scripts/filex.sh` (new)

```bash
#!/bin/bash
set -e

FILEX_HOME="${HOME}/.filex"
FILEX_CURRENT="${FILEX_HOME}/current"

if [ ! -f "${FILEX_CURRENT}/filex" ]; then
    echo "Error: FileX current version not found"
    echo "Please run: filex upgrade"
    exit 1
fi

exec "${FILEX_CURRENT}/filex" "$@"
```

**Tests**: Manual testing on Windows and Unix

## Phase 2: Upgrade Logic (3-4 hours)

### 2.1 Rewrite upgrade_cmd.go

**File**: `cmd/filex/upgrade_cmd.go` (modify existing)

Key changes:
- Remove executable replacement logic
- Add version directory creation
- Use link management for version switching
- Keep checksum verification

```go
func runUpgrade(args []string) error {
    // Parse flags
    checkOnly := false
    // ... parse args ...
    
    // Get current version
    currentVersion, err := version.GetCurrentVersion()
    if err != nil {
        return fmt.Errorf("failed to get current version: %w", err)
    }
    
    // Query GitHub for latest
    latest, err := fetchLatestRelease()
    if err != nil {
        return err
    }
    
    if currentVersion == latest.Version {
        fmt.Printf("Already up-to-date (v%s)\n", currentVersion)
        return nil
    }
    
    if checkOnly {
        fmt.Printf("Update available: v%s -> v%s\n", currentVersion, latest.Version)
        return nil
    }
    
    // Download to versioned directory
    versionDir, err := version.VersionDir(latest.Version)
    if err != nil {
        return err
    }
    
    if err := os.MkdirAll(versionDir, 0755); err != nil {
        return err
    }
    
    binaryPath := filepath.Join(versionDir, binaryName)
    if err := downloadFile(latest.DownloadURL, binaryPath); err != nil {
        return err
    }
    
    // Verify checksum
    if err := verifyChecksum(binaryPath, latest.Checksum); err != nil {
        os.RemoveAll(versionDir)
        return err
    }
    
    // Switch current link
    if err := version.CreateCurrentLink(latest.Version); err != nil {
        os.RemoveAll(versionDir)
        return err
    }
    
    fmt.Printf("Upgraded to v%s\n", latest.Version)
    
    // Automatic cleanup: remove old versions (keep last 3)
    if err := autoCleanup(); err != nil {
        fmt.Printf("Warning: cleanup failed: %v\n", err)
        // Cleanup failure doesn't affect upgrade success
    }
    
    fmt.Println("New version will be used on next invocation")
    return nil
}

// autoCleanup removes old versions, keeping the last 3
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
        if entry.IsDir() {
            versions = append(versions, entry.Name())
        }
    }
    
    // If <= 3 versions, nothing to clean
    if len(versions) <= 3 {
        return nil
    }
    
    // Sort by version (descending, newest first)
    sort.Slice(versions, func(i, j int) bool {
        return versions[i] > versions[j]
    })
    
    // Get current version to avoid removing it
    currentVersion, _ := version.GetCurrentVersion()
    
    // Remove versions beyond the last 3
    removedCount := 0
    for _, v := range versions[3:] {
        if v == currentVersion {
            continue // Never remove current version
        }
        versionDir := filepath.Join(versionsDir, v)
        if err := os.RemoveAll(versionDir); err != nil {
            fmt.Printf("Warning: failed to remove %s: %v\n", v, err)
        } else {
            removedCount++
        }
    }
    
    if removedCount > 0 {
        fmt.Printf("Cleaned up %d old version(s)\n", removedCount)
    }
    
    return nil
}
```

**Tests**: `cmd/filex/upgrade_cmd_test.go`
- Mock GitHub API
- Test download to versioned directory
- Test link switching
- Test checksum verification
- Test rollback on failure
- Test automatic cleanup after upgrade (keeps last 3 versions)
- Test cleanup doesn't remove current version
- Test cleanup failure doesn't affect upgrade success

### 2.2 Add Version Management Commands

**File**: `cmd/filex/upgrade_cmd.go` (extend)

```go
func listVersions() error {
    versionsDir, err := version.VersionsDir()
    if err != nil {
        return err
    }
    
    entries, err := os.ReadDir(versionsDir)
    if err != nil {
        if os.IsNotExist(err) {
            fmt.Println("No versions installed")
            return nil
        }
        return err
    }
    
    currentVersion, _ := version.GetCurrentVersion()
    
    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }
        name := entry.Name()
        if name == currentVersion {
            fmt.Printf("* %s (current)\n", name)
        } else {
            fmt.Printf("  %s\n", name)
        }
    }
    return nil
}

func useVersion(targetVersion string) error {
    versionDir, err := version.VersionDir(targetVersion)
    if err != nil {
        return err
    }
    
    if _, err := os.Stat(versionDir); os.IsNotExist(err) {
        return fmt.Errorf("version v%s not installed", targetVersion)
    }
    
    if err := version.CreateCurrentLink(targetVersion); err != nil {
        return err
    }
    
    fmt.Printf("Switched to v%s\n", targetVersion)
    fmt.Println("New version will be used on next invocation")
    return nil
}

func cleanupVersions() error {
    versionsDir, err := version.VersionsDir()
    if err != nil {
        return err
    }
    
    entries, err := os.ReadDir(versionsDir)
    if err != nil {
        return err
    }
    
    // Collect version directories
    var versions []string
    for _, entry := range entries {
        if entry.IsDir() {
            versions = append(versions, entry.Name())
        }
    }
    
    // Sort by version (newest first)
    sort.Slice(versions, func(i, j int) bool {
        return versions[i] > versions[j]
    })
    
    // Keep last 3, remove rest
    if len(versions) <= 3 {
        fmt.Println("Nothing to clean up")
        return nil
    }
    
    currentVersion, _ := version.GetCurrentVersion()
    
    for _, v := range versions[3:] {
        if v == currentVersion {
            continue // Don't remove current version
        }
        versionDir := filepath.Join(versionsDir, v)
        if err := os.RemoveAll(versionDir); err != nil {
            fmt.Printf("Warning: failed to remove %s: %v\n", v, err)
        } else {
            fmt.Printf("Removed %s\n", v)
        }
    }
    
    return nil
}
```

**Tests**:
- Test listVersions() with various scenarios
- Test useVersion() with valid/invalid versions
- Test cleanupVersions() keeps last 3

## Phase 3: Migration (2-3 hours)

### 3.1 Detect Flat Installation

**File**: `internal/version/migrate.go` (new)

```go
package version

import (
    "os"
    "path/filepath"
    "runtime"
)

// NeedsMigration checks if user has old flat installation
func NeedsMigration() (bool, string, error) {
    // Check if already using versioned structure
    currentLink, err := CurrentLink()
    if err != nil {
        return false, "", err
    }
    
    if _, err := os.Lstat(currentLink); err == nil {
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
    }
    
    // Check PATH locations
    pathDirs := filepath.SplitList(os.Getenv("PATH"))
    for _, dir := range pathDirs {
        searchPaths = append(searchPaths, filepath.Join(dir, exeName))
    }
    
    for _, path := range searchPaths {
        if _, err := os.Stat(path); err == nil {
            return true, path, nil
        }
    }
    
    return false, "", nil
}

// MigrateFromFlat migrates from flat installation to versioned structure
func MigrateFromFlat(oldBinaryPath string) error {
    // Get version from old binary
    // (This is tricky - we might need to run it with --version flag)
    // For now, assume we can extract from buildinfo or prompt user
    
    // Create versions directory
    versionsDir, err := VersionsDir()
    if err != nil {
        return err
    }
    
    if err := os.MkdirAll(versionsDir, 0755); err != nil {
        return err
    }
    
    // For now, use "unknown" version - user can run upgrade to get latest
    versionDir := filepath.Join(versionsDir, "vunknown")
    if err := os.MkdirAll(versionDir, 0755); err != nil {
        return err
    }
    
    // Copy old binary to versioned directory
    binaryName := "filex"
    if runtime.GOOS == "windows" {
        binaryName = "filex.exe"
    }
    
    newBinaryPath := filepath.Join(versionDir, binaryName)
    if err := copyFile(oldBinaryPath, newBinaryPath); err != nil {
        return err
    }
    
    // Create current link
    if err := CreateCurrentLink("unknown"); err != nil {
        return err
    }
    
    return nil
}

func copyFile(src, dst string) error {
    data, err := os.ReadFile(src)
    if err != nil {
        return err
    }
    return os.WriteFile(dst, data, 0755)
}
```

**Tests**: `internal/version/migrate_test.go`
- Test NeedsMigration() with various scenarios
- Test MigrateFromFlat() creates correct structure
- Test migration preserves binary

### 3.2 Integrate Migration into Main

**File**: `cmd/filex/main.go` (modify)

```go
func main() {
    // Check if migration is needed
    needsMigration, oldPath, err := version.NeedsMigration()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Warning: failed to check migration status: %v\n", err)
    } else if needsMigration {
        fmt.Println("Migrating from flat installation to versioned structure...")
        if err := version.MigrateFromFlat(oldPath); err != nil {
            fmt.Fprintf(os.Stderr, "Error: migration failed: %v\n", err)
            os.Exit(1)
        }
        fmt.Println("Migration complete!")
        fmt.Println("Please restart your terminal or run 'filex upgrade' to get the latest version")
    }
    
    // Continue with normal command execution
    // ...
}
```

## Phase 4: Installation (2-3 hours)

### 4.1 Update install.ps1

**File**: `install.ps1` (modify existing)

Key changes:
- Create versioned directory structure
- Download to `versions/v{version}/filex.exe`
- Create `current` junction
- Install wrapper script to PATH

```powershell
# Create directory structure
$versionsDir = Join-Path $InstallDir "versions\v$Version"
New-Item -ItemType Directory -Path $versionsDir -Force | Out-Null

# Download binary to versioned directory
$binaryPath = Join-Path $versionsDir "filex.exe"
Invoke-WebRequest -Uri $downloadUrl -OutFile $binaryPath

# Create current junction
$currentLink = Join-Path $InstallDir "current"
if (Test-Path $currentLink) {
    Remove-Item $currentLink -Force
}
cmd /c mklink /J $currentLink $versionsDir

# Install wrapper script
$wrapperPath = Join-Path $InstallDir "filex.bat"
$wrapperContent = @"
@echo off
setlocal
set FILEX_HOME=$InstallDir
set FILEX_CURRENT=%FILEX_HOME%\current
"%FILEX_CURRENT%\filex.exe" %*
"@
Set-Content -Path $wrapperPath -Value $wrapperContent

# Add wrapper location to PATH
Add-ToPath -Directory $InstallDir
```

### 4.2 Create Unix Install Script

**File**: `install.sh` (new)

```bash
#!/bin/bash
set -e

VERSION=$(curl -s https://api.github.com/repos/BaixuanZhu/FileX/releases/latest | grep tag_name | cut -d '"' -f 4 | sed 's/v//')
INSTALL_DIR="${HOME}/.filex"
VERSIONS_DIR="${INSTALL_DIR}/versions/v${VERSION}"

# Create directory structure
mkdir -p "${VERSIONS_DIR}"

# Download binary
DOWNLOAD_URL="https://github.com/BaixuanZhu/FileX/releases/download/v${VERSION}/filex-v${VERSION}.tar.gz"
curl -L "${DOWNLOAD_URL}" | tar -xz -C "${VERSIONS_DIR}"

# Create current symlink
CURRENT_LINK="${INSTALL_DIR}/current"
rm -f "${CURRENT_LINK}"
ln -s "${VERSIONS_DIR}" "${CURRENT_LINK}"

# Install wrapper script
WRAPPER_PATH="${HOME}/.local/bin/filex"
mkdir -p "$(dirname "${WRAPPER_PATH}")"
cat > "${WRAPPER_PATH}" << 'EOF'
#!/bin/bash
set -e
FILEX_HOME="${HOME}/.filex"
FILEX_CURRENT="${FILEX_HOME}/current"
exec "${FILEX_CURRENT}/filex" "$@"
EOF
chmod +x "${WRAPPER_PATH}"

echo "FileX v${VERSION} installed successfully!"
echo "Wrapper installed to ${WRAPPER_PATH}"
```

## Phase 5: Testing (3-4 hours)

### 5.1 Unit Tests

- `internal/version/paths_test.go`
- `internal/version/link_test.go`
- `internal/version/migrate_test.go`
- `cmd/filex/upgrade_cmd_test.go`

### 5.2 Integration Tests

**File**: `test/integration/upgrade_test.go` (new)

```go
func TestUpgradeFlow(t *testing.T) {
    // Setup: create mock version directory
    // Execute: filex upgrade
    // Verify: new version downloaded, link switched
    // Verify: next invocation uses new version
}

func TestRollbackFlow(t *testing.T) {
    // Setup: install v0.1.2 and v0.1.3
    // Execute: filex upgrade --use v0.1.2
    // Verify: current link points to v0.1.2
}

func TestCleanupFlow(t *testing.T) {
    // Setup: install 5 versions
    // Execute: filex upgrade --cleanup
    // Verify: only 3 versions remain
}

func TestAutoCleanupAfterUpgrade(t *testing.T) {
    // Setup: install 4 versions (v0.1.0, v0.1.1, v0.1.2, v0.1.3)
    // Execute: filex upgrade (upgrades to v0.1.4)
    // Verify: v0.1.4 is current
    // Verify: v0.1.1, v0.1.2, v0.1.3 remain (3 newest)
    // Verify: v0.1.0 is removed
}

func TestAutoCleanupSkipsCurrentVersion(t *testing.T) {
    // Setup: install 5 versions, current is v0.1.2 (not the newest)
    // Execute: filex upgrade --cleanup
    // Verify: v0.1.2 is NOT removed (even if it's beyond the 3-version limit)
}

func TestMigrationFlow(t *testing.T) {
    // Setup: create flat installation
    // Execute: run filex (triggers migration)
    // Verify: versioned structure created
    // Verify: current link points to migrated version
}
```

### 5.3 Platform Tests

**Windows**:
- Test junction creation
- Test wrapper .bat script
- Test migration from flat installation

**Unix**:
- Test symlink creation
- Test wrapper shell script
- Test migration from flat installation

### 5.4 Manual Testing Checklist

- [ ] Fresh install creates correct structure
- [ ] `filex upgrade` downloads and switches version
- [ ] `filex upgrade` automatically cleans up old versions (keeps last 3)
- [ ] `filex upgrade --check` reports available update
- [ ] `filex upgrade --list` shows installed versions
- [ ] `filex upgrade --use <version>` switches version
- [ ] `filex upgrade --cleanup` manually removes old versions
- [ ] Automatic cleanup doesn't remove current version
- [ ] Automatic cleanup failure doesn't affect upgrade success
- [ ] Migration from flat installation works
- [ ] Wrapper script works on Windows
- [ ] Wrapper script works on Unix
- [ ] Checksum verification catches corruption
- [ ] Rollback works when upgrade fails

## Phase 6: Documentation (1-2 hours)

### 6.1 Update README.md

Add section on version management:

```markdown
## Version Management

FileX uses a JVM-style version management system:

- Each version is installed to `~/.filex/versions/v{version}/`
- `~/.filex/current` points to the active version
- Upgrades download new versions and switch the link atomically
- Next invocation automatically uses the new version

### Commands

```bash
# Check for updates
filex upgrade --check

# Upgrade to latest
filex upgrade

# List installed versions
filex upgrade --list

# Switch to specific version
filex upgrade --use v0.1.2

# Clean up old versions (keep last 3)
filex upgrade --cleanup
```

### Migration

If you have an existing flat installation, FileX will automatically migrate it on first run.
```

### 6.2 Update docs/SKILL.md

Add version management commands to quick reference.

### 6.3 Create Migration Guide

**File**: `docs/MIGRATION.md` (new)

Explain:
- What changed (flat → versioned)
- How automatic migration works
- Manual migration steps if needed
- How to rollback if migration fails

## Timeline

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Phase 1: Core Infrastructure | 2-3 hours | None |
| Phase 2: Upgrade Logic | 3-4 hours | Phase 1 |
| Phase 3: Migration | 2-3 hours | Phase 1 |
| Phase 4: Installation | 2-3 hours | Phase 1, 2 |
| Phase 5: Testing | 3-4 hours | All phases |
| Phase 6: Documentation | 1-2 hours | All phases |
| **Total** | **13-19 hours** | |

## Risk Mitigation

1. **Backup before migration**: Copy old binary before moving
2. **Test on Windows VM**: Junction behavior can be tricky
3. **Fallback to copy-based**: If junctions fail, use copy approach
4. **Gradual rollout**: Test with small group before public release
5. **Clear error messages**: Help users understand what went wrong

## Success Criteria

- All unit tests pass
- All integration tests pass
- Manual testing checklist complete
- Documentation updated
- No regression in existing functionality
- Migration works for existing users
- Upgrade takes effect on next invocation
- Rollback works instantly
