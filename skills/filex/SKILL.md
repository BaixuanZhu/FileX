---
name: filex
description: Windows-aware file operations through the filex CLI — encoding-safe read/write/edit (GBK, GB18030, UTF-16, BOM), recoverable recycle-bin deletes, lock diagnosis for undeletable files, registry-backed environment variables, and journaled backups with one-command restore. Use whenever a file might not be UTF-8, a deletion must be recoverable, a file refuses to delete or is locked, environment variables must change persistently on Windows, or an edit needs an audit trail — even if the user never mentions encoding. For plain UTF-8 scratch files, built-in Read/Edit tools are fine.
---

# FileX — Deterministic File Operations for AI Agents

`filex` is a single static binary that wraps file I/O with encoding
detection, an audit journal, and backups. Every command outputs JSON, and
destructive behavior (bypassing the recycle bin, overwriting without backup)
is always opt-in via an explicit flag.

## Step 0 — Install the binary

This skill needs the `filex` CLI on PATH. If `filex version` fails, install it:

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.ps1 | iex
```

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/BaixuanZhu/FileX/main/install.sh | bash
```

Upgrades are self-managed: `filex upgrade --yes` (keeps the last 3 versions).
Alternatives (installer EXE, pre-built archives, `go install`) are documented
in the [FileX README](https://github.com/BaixuanZhu/FileX#installation).

## Optional — register the MCP server

To expose filex as MCP tools in an AI client instead of shell commands:

```bash
filex mcp install --client=<claude|claude-code|codex|cursor|vscode|windsurf|zcode> [--profile=rw]
```

The default `ro` profile exposes read-only tools; `rw` adds write/edit/rm.

## When to Use FileX

| Scenario | Command | Why |
|---|---|---|
| Encoding uncertain (GBK, UTF-16, BOM) | `filex read/write/edit` | Built-in tools assume UTF-8 |
| Delete file/directory | `filex rm` | Default: recycle bin (recoverable) |
| File won't delete (lock?) | `filex lock` | Reports exclusive/deletable status |
| Modify environment variables | `filex env` | Registry + broadcast + 3 scopes |
| Large file tail/grep | `filex tail` / `filex grep --stream` | Memory-efficient, encoding-aware |
| Need audit/rollback | Any op with `--backup` | Journal + backup at `~/.filex/` |
| UTF-8, simple read/edit | Built-in `Read`/`Edit` | Faster, less context |

## Commands

**File**: `read` `write` `edit` `grep` `stat` `lock` `ls` `hash` `rm` `tail`
**Env**: `env get|set|list|unset` (scopes: process|user|machine)
**Backup**: `backup list|restore|prune`
**MCP**: `mcp serve [--profile=ro|rw]` `mcp install --client=<client> [--profile=ro|rw]`
**Upgrade**: `upgrade [--check] [--yes]`

## Key Flags

- `--encoding=auto|utf8|utf8bom|gbk|gb18030|utf16le|utf16be` — force or detect encoding
- `--backup` — snapshot before write/edit
- `--expect-sha256=<hash>` — optimistic lock for concurrent edits
- `--stream` — grep large files without loading all content
- `--purge` — permanently delete (skip recycle bin)
- `--scope=user|machine|process` — env var scope (Windows)

## Examples

```bash
# Edit a GBK config file, keeping a rollback snapshot
filex edit config.ini --find="old" --replace="new" --encoding=gbk --backup

# Safe delete, then undo if needed
filex rm old.txt --yes
filex backup list
filex backup restore <backup-id>

# Tail a huge log without loading it
filex tail app.log --lines=50

# Set a user-scope environment variable (persists, broadcasts the change)
filex env set NODE_ENV production --scope=user

# Upgrade to the latest release
filex upgrade --yes
```

## Safety Model

- **Path policy**: CWD allowed; system dirs denied (Windows, Program Files,
  `.git/objects`); deny patterns always win over allow
- **Journal**: every operation is appended to `~/.filex/journal/filex.jsonl`
  with SHA-256 before/after and sensitive-value masking
- **Deletes** go to the recycle bin unless `--purge` is passed

## Output Format

All output is JSON: `{"success": true, "data": {...}}` or
`{"success": false, "error": {"code": "...", "message": "...", "hint": "..."}}`.
Exit codes: 0 = success, 1 = operation error, 2 = usage error.

---

## Installing this skill (for humans)

```bash
# skills.sh installer — detects 70+ agents and picks the right directory
npx skills add BaixuanZhu/FileX
```

Or copy the `skills/filex/` folder from the
[FileX repo](https://github.com/BaixuanZhu/FileX) into your agent's skills
directory: `~/.agents/skills/` (cross-agent), `~/.zcode/skills/` (ZCode), or
`~/.claude/skills/` (Claude Code).
