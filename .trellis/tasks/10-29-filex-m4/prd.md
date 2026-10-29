# M4: policy + journal + recycle-bin rm

## Goal

Implement path policy (whitelist, deny rules, symlink escape prevention), JSONL audit journal with sensitive value masking, and integrate both into all ops. Deliverable: out-of-bounds paths return E_DENIED; all mutations recorded in journal.

## Requirements

- Path policy: CWD allowed by default, system dirs denied (Windows, Program Files, drive roots, .git/objects)
- Symlink escape prevention: EvalSymlinks then re-check
- Config file: `~/.filex/config.json` with allowed_dirs, deny_patterns, backup_before_write
- Journal: append-only JSONL at `~/.filex/journal/filex.jsonl`
- Journal entries: timestamp, op, path, backup_id, sha256_before, sha256_after
- Sensitive env values masked in journal (PASSWORD/SECRET/KEY/TOKEN/CREDENTIAL → hash only)
- Policy integrated into all file ops (read/write/edit/stat/lock/ls/hash/rm)
- Journal recording for all mutation ops (write/edit/rm/env set/env unset)

## Acceptance Criteria

- [ ] Path under CWD → allowed
- [ ] Path under C:\Windows → E_DENIED
- [ ] Symlink pointing outside allowed → E_DENIED
- [ ] Write/edit/rm creates journal entry
- [ ] Sensitive env values masked in journal
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes

## Notes

- Policy defaults are conservative; config file can extend allowed_dirs
- Journal uses file-level append; no rotation in M4 (defer to M5+)
