# M7: packaging, SKILL.md, README.md

## Goal

Deliver standalone filex.exe with version injection, SKILL.md (<=60 lines) for model consumption, and README.md for human readers. Cross-model evaluation is optional and deferred.

## Requirements

### SKILL.md (docs/SKILL.md)
- <=60 lines, focused on trigger conditions and example commands
- Cover all 14 verbs: read, write, edit, grep, stat, lock, ls, hash, rm, tail, env, backup, mcp, version
- Include "when to use filex vs built-in tools" decision table (from plan §11)
- No syntax templates — only complete example commands (weak models struggle with parameter assembly)
- Trigger conditions: encoding uncertainty, file deletion, env vars, large files, audit/rollback needs

### README.md
- Project overview and motivation
- Installation: `go install` and pre-built binary
- Quick start with 3-5 common scenarios
- Command reference table linking to SKILL.md
- Security model summary (policy, journal, backup)
- MCP integration guide

### Version injection
- Build with ldflags: `-X filex/internal/buildinfo.Version=... -X ...Commit=... -X ...BuildTime=...`
- Add Makefile or build script for reproducible builds
- `filex version` outputs JSON envelope with version/commit/build_time

### Packaging
- `go build -o filex.exe ./cmd/filex` produces standalone binary
- No external runtime dependencies (Windows only, uses stdlib + x/sys)

## Acceptance Criteria

- [ ] `docs/SKILL.md` exists, <=60 lines, covers all verbs with examples
- [ ] `README.md` exists with installation, quick start, command reference
- [ ] `go build -ldflags "..." -o filex.exe ./cmd/filex` injects version info
- [ ] `filex version` returns correct version/commit/build_time in JSON
- [ ] `filex.exe` runs standalone without dependencies

## Notes

- Cross-model evaluation deferred (optional per plan §9.3)
- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight task — PRD-only, no design.md needed.
