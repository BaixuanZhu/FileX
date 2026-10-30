# FileX

English | [简体中文](README.md)

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

Download the latest `.exe` installer from [Releases](https://github.com/BaixuanZhu/FileX/releases) and run it.

The installer will:
- Install FileX to `%LOCALAPPDATA%\FileX`
- Add FileX to your PATH
- Create Start Menu shortcuts

**Silent installation:**
```cmd
<installer>.exe /S
```

**Custom directory:**
```cmd
<installer>.exe /D=D:\Tools\FileX
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

Extract to your PATH. Each archive includes the binary, README, and LICENSE.

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

Output (shape taken from a real run; detection is heuristic, so a GBK file may
be reported as `gb18030` with `probable` confidence):
```json
{
  "schema_version": 1,
  "ok": true,
  "data": {
    "content": "...\n",
    "encoding": { "encoding": "gb18030", "bom": false, "confidence": "probable" },
    "size": 5
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
# Delete to the recovery directory (recoverable)
filex rm old-file.txt

# Preview what a permanent delete would do, without changing anything
filex rm old-file.txt --purge --dry-run

# Permanent delete: type the confirmation in a terminal; refused outright
# when there is no terminal (script, CI, AI agent)
filex rm old-file.txt --purge
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

# Upgrade with no terminal confirmation (audited bypass; refused when guard.allow_bypass=false)
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

See the [filex agent skill](skills/filex/SKILL.md) for complete command reference with examples.

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
| `rm` | Remove (moves to `~/.filex/trash` by default, recoverable) |
| `tail` | Read last N lines (encoding-aware) |

### Environment Variables

| Command | Purpose |
|---|---|
| `env get` | Get variable value (with expansion) |
| `env set` | Set variable (registry + broadcast) |
| `env list` | List variables (sensitive values masked) |
| `env unset` | Delete variable |

### Backups, journal and MCP

| Command | Purpose |
|---|---|
| `backup list` | List backups |
| `backup restore` | Restore from backup |
| `backup prune` | Clean old backups |
| `journal stats` | Aggregate the audit journal: distribution by session/operation/risk class (read-only) |
| `journal path` | Print the audit journal location |
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
- Denied operations are recorded as well, not only the ones that ran

### Backup Strategy

- Write operations with `--backup` create snapshot at `~/.filex/backups/`
- Backup paths use SHA-256 hash of original path (privacy)
- Default retention: 3 backups per file
- `backup prune` cleans old backups

### Recovery directory

- By default `rm` moves files to `~/.filex/trash` (on Windows
  `%USERPROFILE%\.filex\trash`), renamed to `name-timestamp` so you can get them back
- This is filex's own directory, **not the Windows recycle bin**: files do not appear
  in the recycle bin UI and are not touched by its cleanup policy
- `rm --purge` deletes permanently and requires confirmation (see below)
- `rm --dry-run` previews what would happen; it changes nothing on disk and writes no
  audit record

### Confirmation channel and bypass

Irreversible operations are classified by `internal/guard` into four classes:
`purge_delete` (permanent delete), `machine_env_write` (machine-scope environment
variable write), `self_config_write` (writing filex's own config directory
`~/.filex`), `self_update` (upgrade and removal of installed versions).

The decision looks at two things only: **whether the current process has an
interactive terminal**, and **whether this class may be bypassed**.

- Terminal available → prompt y/N; only `y`/`yes` passes. An empty line is a denial.
- No terminal (script, CI, AI agent) → refused outright, no prompt.
- **Confirmation reads the terminal, never stdin**: a `y` fed through a pipe is not a
  human's consent. Even when a process holds `CONIN$`, it is judged non-interactive as
  soon as its stdin is a hijacked pipe — so an agent can neither authorize itself nor
  block forever waiting for a keystroke.
- The MCP path is always non-interactive: a model asking again is not a human who
  approved.

The only way around the confirmation is the explicit flag:

```bash
filex rm big/dir --purge --i-understand-this-is-destructive
```

It is deliberately verbose — adding it should feel like making a decision, not like
completing a command. The audit record keeps `bypass=true` and
`confirm_source=bypass:flag`, so the overruled objection and the executed operation
are both visible. Setting `guard.allow_bypass=false` in `~/.filex/config.json` makes
the flag inert (then the config decides, and a wrongly installed confirmer at the
entry point cannot override it); `guard.enabled=false` is what turns the whole layer
off, which is on by default.

`filex upgrade --yes` keeps the same semantics: it is an audited bypass, no longer
"skip the prompt that actually read stdin". The old `filex rm --yes` is retired — it
used to do nothing, and now reports a usage error.

Denied operations are written to the audit log too (`result=denied`, `decision`,
`decision_reason`); otherwise the journal could not answer "what was stopped".

### Session usage observation

Audit entries may carry `session_id` and `session_source` (`mcp_connection` / `env` /
`absent`) to group the several operations of one task turn, which `filex journal stats`
then aggregates into distributions:

```bash
filex journal stats --since=72h --group-by=session
filex journal stats --json
```

One point must be stated plainly: **`session_id` is an observability label, not a
credential.** It takes part in neither the path policy nor the guard decision, and
forging it only pollutes the forger's own statistics. Therefore:

- Leaving `FILEX_SESSION_ID` unset is the normal state and triggers no behaviour change
- This feature **intercepts nothing, degrades nothing, throttles nothing**; the
  aggregation commands are read-only, and a broken aggregation cannot affect file
  operations
- Old audit records without a session key are byte-for-byte identical to new records
  without one, so existing consumers need no upgrade

`journal stats` reports session key share, per-session p50/p99/max for high-risk
operations, denial and bypass counts, and the MCP hosts it has seen — exactly the data
that decides whether a per-session limit is worth building. The threshold for that
conclusion lives in the task docs; the tool reports measured values and makes no
judgement itself.

### Supply-chain verification

Release artifacts ship with reproducible builds, cosign signatures, SLSA provenance
and SBOM. Verification steps are in [docs/VERIFY.md](docs/VERIFY.md). Reproducibility
is proven in CI by comparing two independent builds; signatures and provenance can
only be accepted in the context of a real release pipeline.

### MCP profiles

- `ro` (default): read-only tools (read, grep, stat, lock, ls, tail, env get/list, backup list)
- `rw`: all tools including write operations

`rw` does not change the decision above: under MCP, `rm --purge`, machine-scope
environment variable writes and upgrades are still refused; it only adds the
capabilities `write`, `edit`, `rm` (the recoverable part of it), `env set/unset`
and `backup restore/prune`.

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

# Claude Code (CLI, user scope)
filex mcp install --client=claude-code

# Codex CLI
filex mcp install --client=codex

# Cursor
filex mcp install --client=cursor

# VS Code (user-level mcp.json)
filex mcp install --client=vscode

# Windsurf
filex mcp install --client=windsurf

# ZCode
filex mcp install --client=zcode
```

This writes the MCP configuration to the client's config directory. Add
`--profile=rw` to register the read-write profile instead of the default
read-only one:

```bash
filex mcp install --client=cursor --profile=rw
```

### Available MCP tools

11 tools mapped to filex operations:
- `read`, `write`, `edit`, `grep`, `stat`, `lock`, `ls`, `rm`, `tail`
- `backup` (actions: list, restore, prune)
- `env` (actions: get, set, list, unset)

Profile enforcement ensures `ro` clients cannot execute write operations.

## Agent Skill

Teach your coding agent when to prefer filex over its built-in tools
(encoding traps, recoverable deletes, locked files, env scopes) with the
distributable skill at [skills/filex/SKILL.md](skills/filex/SKILL.md)
(written in Chinese):

```bash
# skills.sh installer — detects 70+ agents and picks the right directory
npx skills add BaixuanZhu/FileX
```

Or copy the `skills/filex/` folder into your agent's skills directory
(`~/.agents/skills/`, `~/.zcode/skills/`, or `~/.claude/skills/`).

## Output Format

File and environment-variable commands emit a JSON envelope (`help`, `mcp`,
`upgrade` and `journal stats` print human-readable text instead):

**Success**:
```json
{
  "schema_version": 1,
  "ok": true,
  "data": { ... }
}
```

**Error**:
```json
{
  "schema_version": 1,
  "ok": false,
  "error": {
    "code": "E_NOT_FOUND",
    "message": "file not found",
    "hint": "check the path and try again",
    "retryable": false
  }
}
```

When the circuit breaker refuses an operation (`code` is `E_DENIED`), `error`
also carries three fields a caller can branch on instead of parsing prose:

```json
{
  "code": "E_DENIED",
  "message": "denied by guard (purge_delete)",
  "retryable": false,
  "reason": "permanent delete is unrecoverable",
  "safer_alternative": "drop --purge and the file moves to the trash dir, where it can be recovered",
  "guard_class": "purge_delete"
}
```

Exit codes:
- `0`: success
- `1`: internal error
- `2`: usage error
- `3`: refused by the guard
- `4`: file locked
- `5`: not found
- `6`: conflict (for example `--expect-sha256` mismatch)
- `7`: encoding error
- `8`: IO error
- `9`: size limit exceeded
- `10`: unsupported

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
├── skills/
│   └── filex/
│       └── SKILL.md     # Distributable agent skill
├── install.ps1          # Windows installation script
└── install.sh           # Unix installation script
```

## License

MIT

## Contributing

This project follows the Trellis development workflow. See `.trellis/workflow.md` for details.
