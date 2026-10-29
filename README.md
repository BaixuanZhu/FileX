# FileX

Deterministic file and environment variable operations for AI agents on Windows.

## Why FileX?

AI agents need reliable file operations that:
- Preserve encoding (GBK, UTF-16, BOM) — not just UTF-8
- Provide audit trails and rollback capability
- Handle Windows-specific concerns (registry, locks, long paths)
- Prevent dangerous operations (system dirs, concurrent overwrites)

FileX is a CLI tool that wraps these operations in a deterministic, JSON-output interface designed for programmatic consumption by AI models.

## Installation

### Windows Installer (Recommended)

Download `filex-v<version>-windows-x64.exe` (e.g. `filex-v0.3.0-windows-x64.exe`) from [Releases](https://github.com/BaixuanZhu/FileX/releases) and run it.

The installer will:
- Install FileX to `%LOCALAPPDATA%\FileX`
- Add FileX to your PATH
- Create Start Menu shortcuts

**Silent installation:**
```cmd
filex-v0.3.0-windows-x64.exe /S
```

**Custom directory:**
```cmd
filex-v0.3.0-windows-x64.exe /D=D:\Tools\FileX
```

**Moving an existing install:** an explicit `/D=` overrides the previous
location, but the old directory and its PATH entry are left behind —
uninstall first for a clean move.

### PowerShell (Windows, alternative)

```powershell
irm https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.ps1 | iex
```

This downloads the latest release archive, extracts to `%LOCALAPPDATA%\FileX\versions\v{version}\`, creates a `current` junction, and adds the wrapper to your PATH.

### Shell (Linux/macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.sh | bash
```

This downloads the latest release archive, extracts to `~/.filex/versions/v{version}/`, creates a `current` symlink, and adds the wrapper to your PATH.

### Go install (requires Go 1.26+)

```bash
go install github.com/BaixuanZhu/FileX/cmd/filex@latest
```

### Pre-built archives

Download the platform-specific archive from [Releases](https://github.com/BaixuanZhu/FileX/releases):
- `filex-v{version}-windows-x64.zip` (Windows)
- `filex-v{version}-linux-x64.tar.gz` (Linux)
- `filex-v{version}-darwin-x64.tar.gz` (macOS)

Extract to your PATH. Each archive includes the binary, README, LICENSE, and docs.

### Build from source

```bash
# Windows (PowerShell)
$version = git describe --tags --always
$commit = git rev-parse --short HEAD
$buildTime = Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ"
go build -ldflags "-X filex/internal/buildinfo.Version=$version -X filex/internal/buildinfo.Commit=$commit -X filex/internal/buildinfo.BuildTime=$buildTime" -o filex.exe ./cmd/filex

# Windows (Git Bash) / Unix
go build -ldflags "-X filex/internal/buildinfo.Version=$(git describe --tags --always) -X filex/internal/buildinfo.Commit=$(git rev-parse --short HEAD) -X filex/internal/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o filex ./cmd/filex
```

## Quick Start

### Read a file with encoding detection

```bash
filex read config.ini --encoding=auto
```

Output:
```json
{
  "success": true,
  "data": {
    "path": "config.ini",
    "encoding": "gbk",
    "content": "[section]\nkey=value\n",
    "size": 1234,
    "sha256": "abc123..."
  }
}
```

### Edit with backup and rollback

```bash
# Edit with automatic backup
filex edit config.ini --find="old=value" --replace="new=value" --backup

# List backups
filex backup list

# Restore if needed
filex backup restore <backup-id>
```

### Safe deletion

```bash
# Delete to recycle bin (recoverable)
filex rm old-file.txt

# Permanent delete (use with caution)
filex rm old-file.txt --purge --yes
```

### Environment variables (Windows)

```bash
# Set user environment variable
filex env set NODE_ENV production --scope=user

# Read with expansion
filex env get PATH --scope=user

# List all (sensitive values masked)
filex env list --scope=user
```

### Large file operations

```bash
# Tail last 50 lines
filex tail app.log --lines=50

# Stream grep (memory-efficient)
filex grep "ERROR" --paths="*.log" --stream --max=100
```

### Self-upgrade and Version Management

FileX uses a JVM-style version management system where each version is installed to an isolated directory and a `current` link points to the active version.

```bash
# Check if update is available
filex upgrade --check

# Upgrade to latest version (automatic cleanup of old versions)
filex upgrade

# Upgrade without confirmation prompt
filex upgrade --yes

# List all installed versions
filex upgrade --list

# Switch to a specific installed version
filex upgrade --use v0.1.3

# Manually clean up old versions (keeps last 3)
filex upgrade --cleanup
```

**How it works:**
- Downloads platform-specific archive (ZIP for Windows, tar.gz for Unix)
- Verifies SHA-256 checksum
- Extracts to `~/.filex/versions/v{version}/` (or `%LOCALAPPDATA%\FileX\versions\v{version}\` on Windows)
- Atomically switches `current` link to new version
- Automatically removes old versions (keeps last 3)
- Next invocation uses the new version automatically

**Directory structure:**
```
~/.filex/                          # Linux/macOS
%LOCALAPPDATA%\FileX\              # Windows
├── versions/
│   ├── v0.1.2/
│   │   └── filex (or filex.exe)
│   ├── v0.1.3/
│   │   └── filex (or filex.exe)
│   └── v0.2.0/
│       └── filex (or filex.exe)
└── current/                       # Symlink (Unix) or junction (Windows)
    └── -> versions/v0.2.0/
```

**Migration:** If you have an existing flat installation, FileX will automatically migrate it to the versioned structure on first run.

## Command Reference

See [docs/SKILL.md](docs/SKILL.md) for complete command reference with examples.

### File Operations

| Command | Purpose |
|---|---|
| `read` | Read file with encoding detection |
| `write` | Atomic file write |
| `edit` | Byte-level find/replace preserving encoding |
| `grep` | Search with encoding awareness |
| `stat` | File metadata (size, mtime, encoding, lock status) |
| `lock` | Lock diagnosis (exclusive + deletable) |
| `ls` | List directory |
| `hash` | Compute checksum |
| `rm` | Remove (recycle bin by default) |
| `tail` | Read last N lines (encoding-aware) |

### Environment Variables

| Command | Purpose |
|---|---|
| `env get` | Get variable value (with expansion) |
| `env set` | Set variable (registry + broadcast) |
| `env list` | List variables (sensitive values masked) |
| `env unset` | Delete variable |

### Backup & MCP

| Command | Purpose |
|---|---|
| `backup list` | List backups |
| `backup restore` | Restore from backup |
| `backup prune` | Clean old backups |
| `mcp serve` | Start MCP stdio server |
| `mcp install` | Install MCP config for client |

### Upgrade & Version Management

| Command | Purpose |
|---|---|
| `upgrade` | Upgrade to latest version (auto-cleanup old versions) |
| `upgrade --check` | Check for updates without installing |
| `upgrade --list` | List all installed versions |
| `upgrade --use <version>` | Switch to specific installed version |
| `upgrade --cleanup` | Manually remove old versions (keeps last 3) |

## Security Model

### Path Policy

- **CWD allowed**: Current working directory is allowed by default
- **System dirs denied**: `C:\Windows`, `C:\Program Files`, `.git/objects`, etc.
- **Deny overrides allow**: Deny patterns are checked first and always win
- **Symlink protection**: `EvalSymlinks` prevents escape from allowed dirs

### Audit Journal

All operations are logged to `~/.filex/journal/filex.jsonl`:
- Timestamp, operation, path
- SHA-256 before/after for write operations
- Environment variable names (values hashed if sensitive)
- Sensitive names: PASSWORD, SECRET, KEY, TOKEN, CREDENTIAL, PRIVATE, SIGNATURE

### Backup Strategy

- Write operations with `--backup` create snapshot at `~/.filex/backups/`
- Backup paths use SHA-256 hash of original path (privacy)
- Default retention: 3 backups per file
- `backup prune` cleans old backups

### Recycle Bin

- `rm` moves files to Windows recycle bin by default (recoverable)
- `rm --purge` permanently deletes (use with caution)
- `rm --dry-run` shows what would be deleted

### MCP Profiles

- `ro` (default): read-only tools (read, grep, stat, lock, ls, tail, env get/list, backup list)
- `rw`: all tools including write operations

## MCP Integration

filex provides an MCP (Model Context Protocol) server for AI client integration.

### Start MCP server

```bash
filex mcp serve --profile=ro
```

### Install for AI clients

```bash
# Claude Desktop
filex mcp install --client=claude

# Cursor
filex mcp install --client=cursor

# VS Code
filex mcp install --client=vscode
```

This writes the MCP configuration to the client's config directory.

### Available MCP tools

11 tools mapped to filex operations:
- `read`, `write`, `edit`, `grep`, `stat`, `lock`, `ls`, `rm`, `tail`
- `backup` (actions: list, restore, prune)
- `env` (actions: get, set, list, unset)

Profile enforcement ensures `ro` clients cannot execute write operations.

## Output Format

All commands output JSON envelope:

**Success**:
```json
{
  "success": true,
  "data": { ... }
}
```

**Error**:
```json
{
  "success": false,
  "error": {
    "code": "E_NOT_FOUND",
    "message": "file not found",
    "hint": "check the path and try again",
    "retryable": false
  }
}
```

Exit codes:
- `0`: success
- `1`: operation error
- `2`: usage error

## Development

### Build

```bash
go build ./cmd/filex
```

### Test

```bash
# Unit tests (safe to run locally)
go test ./...

# Integration tests (env var registry operations)
# WARNING: These modify Windows registry. Run only in CI or VM.
go test -tags=integration ./internal/winenv/...
```

### Project Structure

```
filex/
├── cmd/filex/           # CLI entry point
├── internal/
│   ├── ops/             # Operation implementations
│   ├── encoding/        # BOM detection, GBK, UTF-16
│   ├── policy/          # Path whitelist, deny rules
│   ├── journal/         # JSONL audit log
│   ├── errs/            # Error codes + hints
│   ├── mcpserver/       # MCP tool schemas
│   ├── version/         # JVM-style version management
│   ├── winenv/          # Windows registry, broadcast
│   ├── winfs/           # Windows filesystem operations
│   └── backup/          # Backup storage, restore
├── scripts/
│   ├── filex.bat        # Windows wrapper script
│   └── filex.sh         # Unix wrapper script
├── docs/
│   └── SKILL.md         # Model usage guide (<=60 lines)
├── install.ps1          # Windows installation script
└── install.sh           # Unix installation script
```

## License

MIT

## Contributing

This project follows the Trellis development workflow. See `.trellis/workflow.md` for details.
