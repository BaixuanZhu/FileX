# M8: self-upgrade, auto-release, installation

## Goal

Add self-upgrade command (from GitHub Releases), auto-release workflow (tag-triggered), and installation methods (PowerShell script, Go install). Deliverable: `filex upgrade` checks and installs latest version; push tag `v*` auto-publishes to GitHub Releases; users can install via PowerShell one-liner or `go install`.

## Requirements

### 1. Auto-release workflow (.github/workflows/release.yml)

**Trigger**: push tag matching `v*` (e.g., `v0.1.0`, `v1.0.0`)

**Build**:
- windows-latest runner
- Inject version from tag: `go build -ldflags "-X filex/internal/buildinfo.Version=${TAG}"`
- Build `FileX.exe` (note: binary name stays lowercase `filex.exe` for compatibility)

**Release**:
- Create GitHub Release with tag name
- Upload `filex.exe` as asset
- Generate and upload `checksums.txt` (SHA-256)
- Auto-generate release notes from commits since last tag

**Optional** (defer if complex):
- Scoop manifest update
- Winget manifest update

### 2. Self-upgrade command

**Usage**:
```bash
filex upgrade           # Check and upgrade to latest
filex upgrade --check   # Only check, don't upgrade
filex upgrade --yes     # Skip confirmation prompt
```

**Flow**:
1. Query GitHub Releases API: `https://api.github.com/repos/BaixuanZhu/FileX/releases/latest`
2. Compare current version (from `buildinfo.Version`) with latest tag
3. If newer version available:
   - Show release notes (optional)
   - Prompt for confirmation (unless `--yes`)
   - Download `filex.exe` from release assets
   - Verify SHA-256 checksum
   - Replace current executable:
     - Windows: rename current exe to `.old`, download new to temp, replace
     - On failure: restore `.old` backup
   - Print success message
4. If already up-to-date: print message and exit 0

**Error handling**:
- Network error: print hint to check internet connection
- Checksum mismatch: abort and print security warning
- Replace failed: restore backup and print error
- Permission denied: hint to run as administrator or use different install location

**Platform considerations**:
- Windows: cannot replace running exe directly
  - Strategy: download to `filex.exe.new`, rename current to `filex.exe.old`, rename new to `filex.exe`, delete old on success
  - On failure: rename old back to current
- Unix: can replace directly (not a priority for now)

### 3. Installation methods

**PowerShell one-liner** (priority):
```powershell
irm https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.ps1 | iex
```

Script behavior:
- Detect latest version from GitHub Releases
- Download `filex.exe` to `$env:LOCALAPPDATA\FileX\filex.exe` (or `$env:USERPROFILE\.FileX\filex.exe`)
- Add to PATH (user scope, no admin required)
- Print success message with usage instructions
- Handle errors gracefully (network, permission, etc.)

**Go install** (already supported):
```bash
go install github.com/lowlighter/filex/cmd/filex@latest
```

**Manual download** (via GitHub Releases page):
- Link in README: https://github.com/BaixuanZhu/FileX/releases

**Scoop** (optional, defer to M9 if needed):
- Requires separate scoop bucket repo
- Manifest file: `filex.json`
- Users: `scoop install filex`

### 4. Documentation updates

**README.md**:
- Add "Installation" section with PowerShell script, Go install, manual download
- Add "Self-upgrade" section with usage examples
- Update branding to FileX

**docs/SKILL.md**:
- Add `upgrade` command to quick reference
- Add installation instructions (brief)

## Acceptance Criteria

- [ ] `.github/workflows/release.yml` exists and triggers on `v*` tags
- [ ] Push tag `v0.1.0` creates GitHub Release with `filex.exe` and `checksums.txt`
- [ ] `filex upgrade --check` queries GitHub and reports if update available
- [ ] `filex upgrade` downloads, verifies, and replaces current executable
- [ ] Upgrade failure restores backup (no broken state)
- [ ] `install.ps1` downloads FileX to user directory and adds to PATH
- [ ] README.md documents all installation methods
- [ ] docs/SKILL.md includes `upgrade` command

## Notes

- Binary name stays `filex.exe` (lowercase) for compatibility
- Brand name is FileX (PascalCase) in documentation only
- Scoop/Winget support deferred to M9 if needed
- Self-upgrade on Unix not a priority (Windows-focused for now)
- GitHub API rate limit: unauthenticated 60 req/hour, authenticated 5000 req/hour
  - Consider caching latest version check result

## Implementation Order

1. Auto-release workflow (M8.1)
2. Installation script (M8.2)
3. Self-upgrade command (M8.3)
4. Documentation updates (M8.4)
