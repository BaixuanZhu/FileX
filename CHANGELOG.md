# Changelog

All notable changes to FileX are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
adheres to [semantic versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-10-30

### Security

- `rm --dry-run` reported a preview while still moving the file into the
  recovery directory. A dry run now changes nothing — not the file, not the
  journal, not the trash directory.
- `upgrade --yes` read its confirmation from `os.Stdin`, so a piped `y` was
  enough for a script or an agent to authorise replacing the running binary.
  The prompt is gone; the flag is now an *audited bypass* of the confirmation
  gate, and it is refused outright when `guard.allow_bypass` is `false`.
- The MCP server no longer lets a tool call authorise itself. Guarded tools
  resolve to a deny confirmer, so `rm --purge` and machine-scope `env` writes
  over MCP are refused rather than silently allowed.
- Destructive operations are gated in one place (`internal/ops.gate`) instead
  of relying on each verb to remember a flag: permanent delete, machine-scope
  environment writes, writes inside FileX's own configuration directory, and
  self-update all require a terminal confirmation from a human.

### Added

- `internal/guard`: a pure decision function over `(Action, policy.Guard,
  Environment)` producing `allow` / `require_confirmation` / `deny`, with the
  reason and a safer alternative attached. No I/O, no global state.
- `guard.enabled` and `guard.allow_bypass` in `policy.json`. Both default to
  true, and a partial block keeps the other default.
- `--i-understand-this-is-destructive` — the explicit bypass flag for
  non-terminal callers. The name is deliberately awkward to type.
- Audit records now carry `actor`, `session_id`, `session_source`,
  `guard_class`, `decision`, `decision_reason`, `confirm_source`, `bypass` and
  `result`. Every new field is `omitempty`, so records written before this
  release serialise byte-for-byte as before (pinned by a compatibility test).
- `FILEX_SESSION_ID` groups separate processes into one session. Values that
  cannot be safely echoed into a JSON line are dropped rather than escaped, so
  the journal stays parseable and the record simply looks unlabelled.
- `filex journal stats` aggregates the audit log read-only — per-session high
  risk counts with p50/p99/max, decision chains per guard class, denials and
  bypasses, and the `--since` / `--until` / `--session` / `--group-by` /
  `--top` filters plus `--json`. Corrupt or truncated lines are skipped, never
  fatal.
- `filex journal path` prints where the log lives without creating it.
- `.github/workflows/ci.yml`: build, vet and test on every push and PR, plus a
  job that builds the same commit twice and fails if the two binaries differ.
- `docs/VERIFY.md`: how to check a downloaded artifact, how to reproduce a
  build, and what is *not* yet provable about a release.

### Changed

- Release and local builds are reproducible: `CGO_ENABLED=0`, `-trimpath`,
  `-buildvcs=false`, `-s -w`, and `BuildTime` derived from the commit's
  committer date instead of the wall clock.
- `LICENSE` is tracked in git with the full MIT text. It previously held a
  one-line placeholder and was untracked, so releases shipped without a
  licence file.

### Breaking

- `filex rm --yes` is retired and returns a usage error. The flag was accepted
  and ignored, which made it look like an authorisation channel that existed.
- New exit code `3` means "refused by the guard" (`E_DENIED`). Scripts that
  only mapped 0/1/2 to outcomes should treat 3 as a refusal, not a retryable
  failure.
- Non-interactive `rm --purge`, `env set/unset --scope=machine` and writes
  into FileX's configuration directory now fail unless a terminal confirms
  them or the bypass flag is supplied and permitted.

## [0.3.0] - 2026-10-29

### Added

- NSIS Windows installer with pre-release CI verification, versioned asset
  names and embedded version metadata.

### Fixed

- Uninstaller removed PATH entries from the wrong positions on the first or
  only install.
- Multi-line changelog text was truncated in GitHub release notes.

## [0.2.0] - 2026-10-29

### Added

- JVM-style version management: multiple installed versions, `upgrade`
  keeps the three most recent, `--list` / `--use` switch between them, and
  releases ship ZIP and tar.gz archives alongside the bare binary.

### Fixed

- Windows-specific sources were missing build tags, so non-Windows builds and
  the `winenv` stub failed to compile.
- Archive filenames did not carry the version.

## [0.1.2] - 2026-10-29

### Fixed

- Release workflow: `ldflags` placeholders were never expanded, so published
  binaries reported an empty version, and the binary filename did not carry it
  either. 0.1.1 and 0.1.2 are the two re-releases that landed these fixes.

## [0.1.0] - 2026-10-29

Initial public release: encoding-aware `read` / `write` / `edit` / `grep` /
`tail`, recoverable `rm`, `lock` and `env` diagnostics, backups with
`backup restore`, an append-only JSONL journal, the MCP stdio server with
`ro` / `rw` profiles, and `filex upgrade` with PowerShell and shell installers.

[Unreleased]: https://github.com/BaixuanZhu/FileX/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/BaixuanZhu/FileX/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/BaixuanZhu/FileX/compare/v0.1.2...v0.2.0
[0.1.2]: https://github.com/BaixuanZhu/FileX/compare/v0.1.0...v0.1.2
[0.1.0]: https://github.com/BaixuanZhu/FileX/releases/tag/v0.1.0
