# M9 Design: JVM-style Upgrade Architecture

## Overview

Transform FileX from a single-binary deployment to a version-managed deployment model, where the executable lives in a versioned directory and a "current" pointer determines which version is active.

## Architecture

### Component Diagram

```
┌─────────────────────────────────────────────────────────┐
│                     User Invocation                      │
│                  `filex <command>`                        │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              Wrapper/Launcher Layer                       │
│  - Resolves `current` link to actual version             │
│  - Executes `~/.filex/current/filex.exe`                 │
│  - Passes through all arguments                          │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              Version Management Layer                     │
│  - ~/.filex/versions/v{version}/filex.exe                │
│  - ~/.filex/current -> versions/v{version}/              │
│  - Atomic link switching                                 │
└─────────────────────────────────────────────────────────┘
```

### Key Design Decisions

#### 1. Wrapper vs Direct Execution

**Option A: Wrapper Script (Recommended)**
- Create `filex.bat` (Windows) / `filex` shell script (Unix) in PATH
- Wrapper resolves `current` link and executes actual binary
- Pros: Clean separation, easy to add pre-flight checks
- Cons: Extra layer, slight performance overhead

**Option B: Binary Reads Link**
- `filex.exe` itself reads `current` link and re-execs if needed
- Pros: No wrapper needed
- Cons: Complex, Windows doesn't support self-re-exec cleanly

**Decision**: Option A - Wrapper script approach

#### 2. Link Type on Windows

**Option A: Directory Junctions**
- `mklink /J current versions\v0.1.3`
- Pros: No admin required, works on all Windows versions
- Cons: Only works for directories, not files

**Option B: File Hard Links**
- `mklink /H current.exe versions\v0.1.3\filex.exe`
- Pros: Works for files
- Cons: Requires admin on older Windows, can't cross volumes

**Option C: Copy-Based**
- Copy `versions\v0.1.3\filex.exe` to `current.exe`
- Pros: Simple, no link issues
- Cons: Not atomic, wastes disk space, slower switch

**Decision**: Option A - Directory junctions (with copy fallback)

#### 3. Migration Strategy

**Scenario**: User has existing `C:\path\to\filex.exe` (flat installation)

**Migration Flow**:
1. On first run, detect if running from `~/.filex/current/`
2. If not, check for `~/.filex/versions/` directory
3. If directory doesn't exist:
   - Create `~/.filex/versions/{current-version}/`
   - Move/copy existing binary there
   - Create `current` junction
   - Update PATH to point to wrapper location
4. Report migration success

**Decision**: Automatic migration on first run

### Data Flow

#### Upgrade Flow

```
1. filex upgrade --check
   └─> Read ~/.filex/current/filex.exe version
   └─> Query GitHub API for latest
   └─> Compare and report

2. filex upgrade
   └─> Download to ~/.filex/versions/v{new}/filex.exe
   └─> Verify checksum
   └─> Remove old `current` junction
   └─> Create new `current` junction -> versions/v{new}/
   └─> Automatic cleanup: remove old versions (keep last 3)
   └─> Report success

3. Next invocation: filex <command>
   └─> Wrapper resolves `current` -> versions/v{new}/
   └─> Executes versions/v{new}/filex.exe
```

#### Rollback Flow

```
1. filex upgrade --use v0.1.2
   └─> Verify ~/.filex/versions/v0.1.2/ exists
   └─> Remove old `current` junction
   └─> Create new `current` junction -> versions/v0.1.2/
   └─> Report success

2. Next invocation uses v0.1.2
```

### File Layout

#### Windows Installation

```
%LOCALAPPDATA%\FileX\
├── filex.bat                    (wrapper script, in PATH)
├── current\                     (junction to active version)
│   └── -> versions\v0.1.3\
└── versions\
    ├── v0.1.2\
    │   └── filex.exe
    └── v0.1.3\
        └── filex.exe
```

#### Unix Installation

```
~/.local/bin/
├── filex                        (wrapper script, in PATH)
├── current -> versions/v0.1.3/  (symlink)
└── versions/
    ├── v0.1.2/
    │   └── filex
    └── v0.1.3/
        └── filex
```

### Packaging Architecture

#### Archive Format Decisions

**Windows**: ZIP format
- Native Windows support (no third-party tools needed)
- `Expand-Archive` PowerShell cmdlet built-in
- Familiar to Windows users

**Linux/macOS**: tar.gz format
- Standard Unix distribution format
- Preserves file permissions (executable bit)
- `tar` and `gzip` universally available

#### Archive Structure

```
filex-v{version}-{platform}.{zip|tar.gz}
└── filex-v{version}-{platform}/          (top-level directory)
    ├── filex.exe (Windows) / filex (Unix)
    ├── README.md
    ├── LICENSE
    └── docs/
        └── SKILL.md
```

**Rationale**:
- Top-level directory prevents file scattering on extraction
- Platform-specific binary naming: `filex.exe` vs `filex`
- Docs bundled for offline access and AI agent integration

#### Multi-Platform Build Strategy

**GitHub Actions matrix build**:
```yaml
strategy:
  matrix:
    include:
      - os: windows-latest
        goarch: amd64
        archive_ext: zip
        binary_name: filex.exe
      - os: ubuntu-latest
        goarch: amd64
        archive_ext: tar.gz
        binary_name: filex
      - os: macos-latest
        goarch: amd64
        archive_ext: tar.gz
        binary_name: filex
```

**Build steps per platform**:
1. Cross-compile Go binary with version injection
2. Create platform-specific directory structure
3. Copy binary + docs + license
4. Package as ZIP (Windows) or tar.gz (Unix)
5. Generate SHA-256 checksum
6. Upload to GitHub Releases

#### Release Workflow Updates

**Trigger**: `v*` tags (existing)

**New artifacts**:
- `filex-v{version}-windows-x64.zip`
- `filex-v{version}-linux-x64.tar.gz`
- `filex-v{version}-darwin-x64.tar.gz`
- `checksums.txt` (all archives)

**Removed artifacts**:
- Standalone `filex.exe` (replaced by ZIP)
- Platform-named binaries without version (replaced by versioned archives)

#### Upgrade Flow with Archives

```
1. Query GitHub Releases API for latest version
2. Determine platform-specific archive name:
   - Windows: filex-v{version}-windows-x64.zip
   - Linux: filex-v{version}-linux-x64.tar.gz
   - macOS: filex-v{version}-darwin-x64.tar.gz
3. Download archive to temporary directory
4. Verify SHA-256 checksum against checksums.txt
5. Extract archive:
   - ZIP: PowerShell Expand-Archive (Windows)
   - tar.gz: tar -xzf (Unix)
6. Extracted directory: filex-v{version}-{platform}/
7. Move binary to ~/.filex/versions/v{version}/
8. Copy docs to ~/.filex/versions/v{version}/docs/
9. Switch current link to new version
10. Cleanup temporary files
```

#### Install Script Updates

**Windows (install.ps1)**:
- Download ZIP archive
- Verify checksum
- Extract to temporary directory
- Create ~/.filex/versions/v{version}/ structure
- Move files from extracted archive
- Create current junction
- Create wrapper script (filex.bat)
- Add to PATH

**Unix (install.sh)** - new script:
- Download tar.gz archive
- Verify checksum
- Extract to temporary directory
- Create ~/.local/bin/versions/v{version}/ structure
- Move files from extracted archive
- Create current symlink
- Create wrapper script (filex)
- Add to PATH (if not already present)

### Error Handling

#### Junction Creation Fails

**Scenario**: Windows junction creation fails (rare, but possible)

**Fallback**: Copy-based approach
1. Copy `versions/v{new}/filex.exe` to `current.exe`
2. Rename `current.exe` to `current.exe.old` (atomic on Windows)
3. Rename new `current.exe.tmp` to `current.exe` (atomic)
4. Delete `current.exe.old`

**Detection**: Try junction first, catch error, use fallback

#### Version Directory Corrupted

**Scenario**: `versions/v0.1.3/filex.exe` is corrupted or missing

**Handling**:
1. Verify checksum before switching `current` link
2. If verification fails, don't switch, report error
3. If `current` link points to missing version, report error and suggest re-install

#### Disk Full During Download

**Scenario**: Insufficient disk space to download new version

**Handling**:
1. Check available space before download
2. If <50MB free, abort with clear error message
3. Suggest running `filex upgrade --cleanup` to free space

### Security Considerations

#### Checksum Verification

- Always verify SHA-256 before switching `current` link
- Checksum file downloaded from GitHub Releases (trusted source)
- Mismatch = abort upgrade, don't modify `current`

#### Link Manipulation

- Only create/modify links in `~/.filex/` directory
- Never follow links outside this directory tree
- Validate link target before execution

#### Version Directory Permissions

- `~/.filex/versions/` should be writable only by current user
- Reject versions with suspicious permissions
- On Windows, inherit parent directory ACLs

### Testing Strategy

#### Unit Tests

- Version comparison logic
- Link creation/deletion (mock filesystem)
- Checksum verification
- Migration detection

#### Integration Tests

- Full upgrade flow (download → verify → switch)
- Rollback flow
- Cleanup flow
- Migration from flat installation

#### Platform Tests

- Windows: junction creation, wrapper .bat script
- Unix: symlink creation, wrapper shell script
- Both: PATH resolution, version switching

### Performance Impact

#### Startup Time

- Wrapper adds ~10-50ms overhead (link resolution)
- Acceptable for CLI tool (typical operations take seconds)

#### Disk Usage

- ~10MB per version
- Default: keep last 3 versions = ~30MB total
- Acceptable for modern systems

#### Upgrade Time

- Download: network-bound (unchanged)
- Verification: ~100ms for 10MB file (negligible)
- Link switch: ~10ms (instant)
- Total: dominated by download speed

### Backward Compatibility

#### CLI Interface

- All existing commands work unchanged
- `filex upgrade` behavior changes (better)
- New flags: `--list`, `--use`, `--cleanup`

#### MCP Server

- No changes to MCP protocol
- Server binary location changes (wrapper handles it)
- MCP clients see no difference

#### Scripts/Automation

- Scripts calling `filex` continue to work
- PATH points to wrapper, not binary directly
- Wrapper is drop-in replacement

### Future Enhancements (Out of Scope for M9)

- Version pinning per project (`.filex-version` file)
- Automatic cleanup of old versions
- GUI for version management
- Remote version management (multiple machines)
- Beta/canary channel support

## Implementation Phases

### Phase 1: Core Infrastructure
- Define version directory structure
- Implement link management (junction/symlink)
- Create wrapper scripts (Windows/Unix)

### Phase 2: Upgrade Logic
- Rewrite `upgrade_cmd.go` to use versioned directories
- Implement atomic link switching
- Add checksum verification

### Phase 3: Version Management Commands
- Implement `--list`, `--use`, `--cleanup` flags
- Add version discovery and validation

### Phase 4: Migration
- Detect flat installation
- Implement automatic migration
- Test migration scenarios

### Phase 5: Installation
- Update `install.ps1` for new structure
- Create Unix install script
- Update documentation

### Phase 6: Testing
- Unit tests for all components
- Integration tests for full flows
- Platform-specific tests (Windows/Unix)
- Migration tests

## Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Junction creation fails on Windows | High | Low | Copy-based fallback |
| Migration corrupts existing install | High | Low | Backup before migration, rollback on failure |
| Wrapper adds too much overhead | Medium | Low | Benchmark, optimize if needed |
| Users confused by new directory structure | Medium | Medium | Clear documentation, migration guide |
| Link manipulation security issue | High | Low | Validate all paths, checksum verification |

## Conclusion

This design transforms FileX into a professional-grade tool with robust version management, matching the quality of established tools like sdkman and nvm. The implementation is straightforward, the risks are manageable, and the user experience improvement is significant.
