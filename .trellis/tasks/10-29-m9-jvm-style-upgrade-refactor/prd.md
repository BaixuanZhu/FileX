# M9: JVM-style Upgrade Refactor

## Goal

Refactor FileX self-upgrade mechanism from "replace running executable" to "JVM-style version management" with multi-version directory structure and atomic version switching.

## Background

Current implementation (M8) modifies the running executable during upgrade, which:
- Risks file locking issues on Windows
- Requires terminal restart to use new version
- Doesn't support multi-version coexistence
- Has complex rollback logic with .old files

JVM-style approach (used by sdkman, nvm, rustup):
- Downloads new version to isolated directory
- Atomically switches "current" pointer
- Next invocation uses new version automatically
- Supports instant rollback to previous versions

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
