# M9: JVM-style Upgrade Refactor

## Goal

Refactor FileX self-upgrade mechanism from "replace running executable" to "JVM-style version management" with multi-version directory structure, atomic version switching, and archive-based distribution (ZIP/tar.gz).

## Background

Current implementation (M8) modifies the running executable during upgrade, which:
- Risks file locking issues on Windows
- Requires terminal restart to use new version
- Doesn't support multi-version coexistence
- Has complex rollback logic with .old files
- Distributes as standalone binary (no bundled docs/license)

JVM-style approach (used by sdkman, nvm, rustup):
- Downloads new version to isolated directory
- Atomically switches "current" pointer
- Next invocation uses new version automatically
- Supports instant rollback to previous versions
- Distributes as platform-specific archives (ZIP/tar.gz)

## Requirements

### Directory Structure

```
~/.filex/
├── versions/
│   ├── v0.1.2/
│   │   └── filex.exe
│   ├── v0.1.3/
│   │   └── filex.exe
│   └── v0.1.4/
│       └── filex.exe
└── current/          (symbolic link or junction to active version)
    └── -> versions/v0.1.3/
```

### Upgrade Flow

1. Query GitHub Releases API for latest version
2. Compare current version (from `current/filex.exe`) with latest
3. If newer version available:
   - Download to `versions/v{new}/filex.exe`
   - Verify SHA-256 checksum
   - Atomically switch `current` link to new version
   - Report success (new version available on next run)
4. If already up-to-date: report and exit

### Version Switching Strategy

**Windows**:
- Use directory junctions (no admin required)
- `mklink /J current versions\v0.1.3`
- Fallback: copy-based approach if junctions fail

**Unix/Linux/macOS**:
- Use symbolic links
- `ln -sfn versions/v0.1.3 current`

### Commands

- `filex upgrade` - Check and upgrade to latest (auto-cleanup old versions, keep last 3)
- `filex upgrade --check` - Only check, don't upgrade
- `filex upgrade --list` - List installed versions
- `filex upgrade --use <version>` - Switch to specific version
- `filex upgrade --cleanup` - Manually remove old versions (keep last 3)

### Automatic Cleanup

After successful upgrade:
- Automatically remove old versions (keep last 3)
- Never remove current active version
- Cleanup failure doesn't affect upgrade success (warning only)
- Reduces disk usage (~10MB per version)

### Migration

Existing users with flat `filex.exe` installation:
- Detect old installation on first run
- Move existing binary to `versions/v{current}/filex.exe`
- Create `current` link
- Report migration success

### Rollback

- `filex upgrade --use v0.1.2` switches `current` link
- Instant, no download needed
- Old versions remain until explicitly cleaned up

### Distribution Format

**Archive-based distribution** (replaces standalone binary):

**Release artifacts**:
```
filex-v{version}-windows-x64.zip       # Windows portable
filex-v{version}-linux-x64.tar.gz      # Linux portable
filex-v{version}-darwin-x64.tar.gz     # macOS portable
checksums.txt                          # SHA-256 for all archives
```

**Archive contents**:
```
filex-v{version}-{platform}/
├── filex.exe (or filex on Unix)       # Main binary
├── README.md                          # Documentation
├── LICENSE                            # License file
└── docs/
    └── SKILL.md                       # AI agent usage guide
```

**Benefits**:
- Bundles binary + docs + license
- Platform-specific naming
- Easier checksum verification
- Consistent with JVM ecosystem tools
- Simplifies upgrade logic (download → extract → install)

**Upgrade flow update**:
1. Download platform-specific archive (e.g., `filex-v0.1.3-windows-x64.zip`)
2. Verify SHA-256 checksum
3. Extract to temporary directory
4. Move binary to `versions/v{version}/`
5. Copy docs to `versions/v{version}/docs/`
6. Switch `current` link
7. Cleanup temporary files

## Acceptance Criteria

- [ ] New directory structure: `~/.filex/versions/v{version}/filex.exe`
- [ ] `current` link points to active version (junction on Windows, symlink on Unix)
- [ ] `filex upgrade` downloads to versioned directory, switches link
- [ ] `filex upgrade` automatically cleans up old versions (keeps last 3)
- [ ] No modification of running executable
- [ ] Next invocation after upgrade uses new version automatically
- [ ] `filex upgrade --list` shows all installed versions
- [ ] `filex upgrade --use <version>` switches to existing version
- [ ] `filex upgrade --cleanup` manually removes old versions (keeps last 3)
- [ ] Migration from flat installation works seamlessly
- [ ] Install script (`install.ps1`) creates proper directory structure
- [ ] All existing tests pass
- [ ] New tests for version management logic
- [ ] New tests for automatic cleanup after upgrade
- [ ] Release workflow builds multi-platform archives (Windows ZIP, Linux tar.gz, macOS tar.gz)
- [ ] Each archive contains: binary, README.md, LICENSE, docs/SKILL.md
- [ ] Archive filenames include version and platform (e.g., `filex-v0.1.3-windows-x64.zip`)
- [ ] SHA-256 checksums generated for all archives
- [ ] `filex upgrade` downloads and extracts platform-specific archive
- [ ] Install script (`install.ps1`) handles archive extraction and installation
- [ ] Unix install script (`install.sh`) created for Linux/macOS

## Constraints

- Must work on Windows without admin privileges (use junctions, not symlinks)
- Must maintain backward compatibility with existing CLI interface
- Must not break existing MCP server functionality
- Migration must be automatic and transparent to users

## Out of Scope

- GUI for version management
- Remote version management (multiple machines)
- Version pinning per project
- Configurable retention count (hardcoded to 3)

## Success Metrics

- Zero file locking issues during upgrade
- Upgrade takes effect on next invocation (no terminal restart)
- Rollback to previous version takes <1 second
- Disk usage: ~10MB per version (acceptable for CLI tool)
