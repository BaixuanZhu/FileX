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

### PowerShell (recommended)

```powershell
irm https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.ps1 | iex
```

This downloads the latest release to `%LOCALAPPDATA%\FileX\` and adds it to your PATH.

### Go install (requires Go 1.26+)

```bash
go install github.com/BaixuanZhu/FileX/cmd/filex@latest
```

### Pre-built binary

Download `filex.exe` from [Releases](https://github.com/BaixuanZhu/FileX/releases) and add to your PATH.

### Build from source

```bash
# Windows (PowerShell)
$version = git describe --tags --always
$commit = git rev-parse --short HEAD
$buildTime = Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ"
go build -ldflags "-X filex/internal/buildinfo.Version=$version -X filex/internal/buildinfo.Commit=$commit -X filex/internal/buildinfo.BuildTime=$buildTime" -o filex.exe ./cmd/filex

# Windows (Git Bash)
go build -ldflags "-X filex/internal/buildinfo.Version=$(git describe --tags --always) -X filex/internal/buildinfo.Commit=$(git rev-parse --short HEAD) -X filex/internal/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o filex.exe ./cmd/filex
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

### Self-upgrade

```bash
# Check if update is available
filex upgrade --check

# Upgrade to latest version
filex upgrade

# Upgrade without confirmation prompt
filex upgrade --yes
```

FileX checks [GitHub Releases](https://github.com/BaixuanZhu/FileX/releases) for the latest version, downloads it, verifies the checksum, and replaces the current executable. If the upgrade fails, it automatically restores the previous version.

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
| `upgrade` | Upgrade to latest version |

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
│   ├── winenv/          # Windows registry, broadcast
│   └── backup/          # Backup storage, restore
└── docs/
    └── SKILL.md         # Model usage guide (<=60 lines)
```

## License

MIT

## Contributing

This project follows the Trellis development workflow. See `.trellis/workflow.md` for details.
