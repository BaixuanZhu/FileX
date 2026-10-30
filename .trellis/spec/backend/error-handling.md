# Error Handling

> How errors are handled in this project.

---

## Overview

FileX is a single-binary CLI whose primary caller is often another program
(an agent, a script, an MCP host). Errors are therefore a **machine-readable
contract**, not a message printed for a human:

- Every layer below `cmd/filex` returns `*errs.Error`, never a bare `error`.
- `cmd/filex/main.go` turns the returned value into a JSON envelope on
  **stdout** and exits with `err.Code.ExitCode()`.
- Nothing on the guarded path writes an error to stderr and keeps going; a
  caller that cannot see the failure is worse than a loud one.

---

## Error Types

`internal/errs/errs.go` defines one struct (`Error`) and a closed set of codes.
`Code.ExitCode()` has an explicit case per code and a `default` that falls back
to `1` — so a new code without its own case silently becomes "operation error"
rather than failing loudly. Add the case in the same change as the code.

| Code | Exit | Meaning |
|---|---|---|
| `E_INTERNAL` | 1 | unexpected failure |
| `E_USAGE` | 2 | bad verb, bad flag, retired flag |
| `E_DENIED` | 3 | refused by the guard (circuit breaker) |
| `E_LOCKED` | 4 | file held by another process |
| `E_NOT_FOUND` | 5 | path missing |
| `E_CONFLICT` | 6 | optimistic lock mismatch (`--expect-sha256`) |
| `E_ENCODING` | 7 | encoding could not be determined or honoured |
| `E_IO` | 8 | OS-level read/write failure |
| `E_TOO_LARGE` | 9 | size limit exceeded |
| `E_UNSUPPORTED` | 10 | operation not available on this platform |

---

## The Envelope

```go
errs.WriteSuccess(w, data)   // {"schema_version":1,"ok":true,"data":{...}}
errs.WriteFailure(w, err)    // {"schema_version":1,"ok":false,"error":{...}}
```

Exceptions that print human text instead: `help`, `mcp`, `upgrade`,
`journal stats` (the last one has `--json`). Keep new commands inside the
envelope convention unless there is a real reason not to.

---

## Guard Refusals

A refusal carries structured fields so the caller can branch instead of
parsing prose:

```go
return errs.DeniedByGuard(msg, class, reason, saferAlternative)
```

which serialises as `guard_class`, `reason` and `safer_alternative`.

- Build refusal text with `DeniedByGuard`, never by formatting a sentence into
  `Message` and hoping an agent reads it.
- `safer_alternative` must name a command that actually works
  (e.g. "drop `--purge`"), not "consider whether you really want to do this".

---

## Forbidden Patterns

- **Returning `error` from `internal/ops`** — it loses the code and the exit
  status degrades to `E_INTERNAL`.
- **Defaulting an unknown refusal to allow.** When a confirmer, a terminal or
  a decision cannot be determined, the answer is *no*.
- **Retiring a flag by ignoring it.** `rm --yes` was accepted and did nothing;
  removing it now returns `E_USAGE` naming the flag. Silent acceptance teaches
  callers that the flag is a no-op.
- **Mapping a refusal to a retryable failure.** `E_DENIED` means a human has
  to decide; `retryable` stays `false`.

---

## Common Mistakes

- Emitting the envelope to stderr. It goes to stdout; only the plain-text
  usage banner uses stderr.
- Forgetting `WithHint`/`WithRetryable` when a failure is genuinely transient,
  which pushes agents into blind retry loops.
