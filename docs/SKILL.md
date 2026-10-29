# filex — Deterministic File Operations for AI Agents

## When to Use filex

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
**MCP**: `mcp serve [--profile=ro|rw]` `mcp install --client=claude|cursor|vscode`

## Key Flags

- `--encoding=auto|utf8|gbk|gb18030|utf16le|utf16be` — force or detect encoding
- `--backup` — snapshot before write/edit
- `--expect-sha256=<hash>` — optimistic lock for concurrent edits
- `--stream` — grep large files without loading all content
- `--purge` — permanently delete (skip recycle bin)
- `--scope=user|machine|process` — env var scope (Windows)

## Examples

```bash
# Edit non-UTF-8 file with backup
filex edit config.ini --find="old" --replace="new" --encoding=gbk --backup

# Safe delete with rollback
filex rm old.txt --yes
filex backup list
filex backup restore <backup-id>

# Large file tail
filex tail app.log --lines=50

# Set environment variable (Windows, user scope)
filex env set NODE_ENV production --scope=user
```

## Security

- **Policy**: CWD allowed; system dirs denied (Windows, Program Files, .git/objects)
- **Deny > allow**: deny patterns always win
- **Journal**: `~/.filex/journal/filex.jsonl` with sensitive value masking
- **MCP profiles**: `ro` (default) = read-only; `rw` = all tools

## Output Format

All output is JSON: `{"success": true/false, "data": {...}}` or `{"success": false, "error": {"code": "...", "message": "...", "hint": "..."}}`

Exit codes: 0=success, 1=operation error, 2=usage error
