# Logging Guidelines

> How logging is done in this project.

---

## Overview

FileX has no console logger and no levels. It has **one append-only audit
journal**: `$HOME/.filex/journal/filex.jsonl`, written by `internal/journal`.
Every operation — including refused ones — appends exactly one JSON line.

There is deliberately no `slog`/`logrus`, no debug output, no rotation. A file
that records what happened to a user's files is evidence, not diagnostics:
its format is a compatibility surface, not an implementation detail.

---

## Structured Logging

The single schema is `journal.Entry` (`internal/journal/journal.go`). Readers
must not be written against anything else.

- `journal stats` (`internal/journal/stats.go`) deserialises the same struct.
  **Never hand-write a second parser** — a divergent field name silently drops
  records from the aggregation.
- Writes are serialised under a package mutex and opened with
  `O_APPEND | O_CREATE`.
- `Path()` resolves the location **without creating the directory**; only the
  write path creates it. A read that materialises `~/.filex/journal/` on a
  fresh install is a bug (pinned by `TestCollectWithoutALog`).

---

## Adding a Field

Rules, in order:

1. The field is `omitempty`. Records written before a change must serialise
   **byte-for-byte** as before — `internal/journal/compat_test.go` asserts this
   and will fail the build otherwise.
2. Anything that reaches a JSON line from outside (env vars, client names,
   session ids) must be validated, not escaped. `FILEX_SESSION_ID` is the
   precedent: an unusable value is **dropped**, which yields the same shape as
   "no session at all". Repairing it (trimming, replacing newlines) would put
   attacker-chosen text into the log.
3. A value that means "absent" is represented by the **empty** field, not by a
   literal `"absent"` string. Readers normalise empty → `absent`.

---

## What to Log

- Operation, absolute path, and the SHA-256 before and after a write.
- Environment variable **names**, scope, and a hash of the value.
- The full guard decision chain: `actor`, `session_id`, `session_source`,
  `guard_class`, `decision`, `decision_reason`, `confirm_source`, `bypass`,
  `result`.
- Denials. A log of only what succeeded cannot answer what was blocked, and
  "did the breaker actually fire?" is the first question asked of it.
- Bypasses with their source, so a flag-driven run is distinguishable from a
  human typing `y`.

---

## What NOT to Log

Never write these in clear text:

- Environment variable **values**. Names matching `PASSWORD`, `SECRET`, `KEY`,
  `TOKEN`, `CREDENTIAL`, `PRIVATE`, `SIGNATURE` are treated as sensitive and
  reduced to a SHA-256.
- File contents. Record digests, not payloads.
- Backup locations that would leak the original path — backups are keyed by a
  hash of the source path for this reason.
- Raw MCP client blobs. `client_info` is a short label, not the whole
  `initialize` payload.

---

## Forbidden Patterns

- Calling the aggregation (`stats.Collect` / `Scan`) from the **write** path.
  Observability must not be able to block a file operation.
- Reading the journal with a lock or assuming it is complete mid-write. It is
  append-only; a truncated tail line is a normal state, and `journal stats`
  skips malformed lines and still exits `0`.
- Making a read command create directories or touch the log file.
