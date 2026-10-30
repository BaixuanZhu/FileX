# Guard Patterns

> Circuit-breaker decision layer for irreversible operations.

---

## Overview

FileX has a **guard layer** that sits between policy check and execution. It provides an independent confirmation channel for high-risk operations, separate from the model's authorization. The guard is a pure-function decision layer (`internal/guard`) that classifies operations and returns structured verdicts.

**Key insight**: The guard is not a permission system — it's a "after permission passes, before execution" circuit breaker. Policy (ro/rw profile, path rules) is the first gate; guard is the second.

---

## Architecture

```
checkPolicy → guard.Evaluate → Confirmer.Confirm → execute → journal
```

All file operations converge at `internal/ops` and pass through this pipeline. The guard never reads files, environment variables, or terminal state — it's a pure function of `(Action, Environment, Config)`.

---

## Classifications

Four hardcoded classifications (v2 does not use a rule engine):

| Class | Trigger | Example |
|---|---|---|
| `purge_delete` | Irreversible deletion | `rm --purge` |
| `machine_env_write` | System-wide environment change | `env set --scope machine` |
| `self_config_write` | Writing into FileX's own config | `write ~/.filex/config.json` |
| `self_update` | Replacing the FileX binary | `filex upgrade` |

Everything else returns `allow`. Classifications are **not configurable** — adding a 5th–6th is fine; beyond that, introduce a rule engine (→ backlog B3).

---

## Signatures

### Decision Layer

```go
// internal/guard/guard.go

type Verb string
const (
    VerbRm     Verb = "rm"
    VerbWrite  Verb = "write"
    VerbEdit   Verb = "edit"
    VerbEnvSet Verb = "env_set"
    VerbEnvUnset Verb = "env_unset"
    VerbUpgrade Verb = "upgrade"
)

type Action struct {
    Verb      Verb
    Path      string   // absolute, after EvalSymlinks
    Scope     string   // "user" | "machine" (env only)
    EnvName   string   // env var name (env only)
    Bypass    bool     // explicit --i-understand-this-is-destructive
}

type Environment struct {
    Interactive bool   // can we open a real terminal?
    Actor       string // "cli" | "mcp:<clientName>"
    SessionID   string
}

type Config struct {
    Enabled     bool   // guard.enabled (default true)
    AllowBypass bool   // guard.allow_bypass (default true)
}

type Outcome string
const (
    OutcomeAllow             Outcome = "allow"
    OutcomeRequireConfirmation Outcome = "require_confirmation"
    OutcomeDeny              Outcome = "deny"
)

type Decision struct {
    Outcome          Outcome
    Class            string   // e.g. "purge_delete"
    Reason           string   // human-readable
    SaferAlternative string   // command suggestion
}

func Evaluate(action Action, env Environment, cfg Config) Decision
```

### Confirmer Interface

```go
// internal/guard/confirmer.go

type Confirmer interface {
    Confirm(decision Decision) (source string, err error)
}

// source is one of: "tty", "bypass", "none"
```

Three implementations:

| Confirmer | When to Use | Behavior |
|---|---|---|
| `TTYConfirmer` | CLI with real terminal | Opens `CONIN$` (Windows) or `/dev/tty` (Unix); prompts `y/N`; returns `source="tty"` on accept |
| `DenyConfirmer` | MCP, non-interactive CLI | Always denies; returns `source="none"` |
| `BypassConfirmer` | `--i-understand-this-is-destructive` flag | Accepts without prompt; returns `source="bypass"`; **must audit `bypass=true`** |

### Console Detection

```go
// internal/console/console.go

var ErrNoTerminal = errors.New("no real terminal available")

func OpenInput() (*os.File, error)
// Windows: opens \\.\CONIN$
// Unix: opens /dev/tty
// Returns ErrNoTerminal if open fails or stdin is not a character device
```

**Critical**: `OpenInput` checks both that the console device can be opened **and** that stdin itself is a character device. An agent-spawned process with piped stdin will fail the second check even if `CONIN$` opens successfully.

---

## Contracts

### Request (Action)

| Field | Type | Constraints |
|---|---|---|
| `Verb` | `Verb` | one of the 6 constants |
| `Path` | `string` | absolute, after `EvalSymlinks` |
| `Scope` | `string` | `"user"` or `"machine"` (env ops only) |
| `EnvName` | `string` | env var name (env ops only) |
| `Bypass` | `bool` | true only if explicit bypass flag present |

### Response (Decision)

| Field | Type | Constraints |
|---|---|---|
| `Outcome` | `Outcome` | `allow` / `require_confirmation` / `deny` |
| `Class` | `string` | classification name or empty |
| `Reason` | `string` | human-readable explanation |
| `SaferAlternative` | `string` | command suggestion (must be actionable) |

### Environment

| Field | Type | Source |
|---|---|---|
| `Interactive` | `bool` | `console.OpenInput() != ErrNoTerminal` |
| `Actor` | `string` | `"cli"` or `"mcp:<clientName>"` |
| `SessionID` | `string` | from `FILEX_SESSION_ID` env or MCP connection |

### Audit Fields (journal.Entry extensions)

| Field | Type | omitempty | Meaning |
|---|---|---|---|
| `guard_class` | `string` | yes | classification that fired |
| `decision` | `string` | yes | `allow` / `deny` |
| `decision_reason` | `string` | yes | human-readable |
| `confirm_source` | `string` | yes | `tty` / `bypass` / `none` |
| `bypass` | `bool` | yes | true if bypass flag used |
| `result` | `string` | yes | `allowed` / `denied` / `bypassed` |
| `actor` | `string` | yes | who initiated |
| `session_id` | `string` | yes | connection-level session |
| `session_source` | `string` | yes | `env` / `mcp` / empty |

All fields are `omitempty` — old journal consumers see no change.

---

## Validation & Error Matrix

| Condition | Outcome | Error |
|---|---|---|
| `guard.enabled=false` | `allow` | — |
| Non-high-risk operation | `allow` | — |
| High-risk + interactive + user accepts | `allow` | — |
| High-risk + interactive + user refuses | `deny` | `E_DENIED` with `reason` + `safer_alternative` |
| High-risk + non-interactive + no bypass | `deny` | `E_DENIED` |
| High-risk + bypass flag + `allow_bypass=true` | `allow` | audit `bypass=true` |
| High-risk + bypass flag + `allow_bypass=false` | `deny` | `E_DENIED` |
| MCP + any high-risk | `deny` | `E_DENIED` (MCP is never interactive) |

---

## Good / Base / Bad Cases

### Good: CLI with real terminal

```bash
$ filex rm important.txt --purge
# TTYConfirmer prompts: "This will permanently delete. Continue? [y/N]"
$ y
# Operation executes; audit: guard_class=purge_delete, decision=allow, confirm_source=tty
```

### Base: Non-interactive CLI

```bash
$ echo "y" | filex rm important.txt --purge
# console.OpenInput() detects piped stdin → ErrNoTerminal
# DenyConfirmer denies; exit 3; audit: decision=deny, confirm_source=none
```

### Bad: MCP self-authorization (prevented)

```json
// MCP tool call
{"method": "rm", "params": {"path": "important.txt", "purge": true, "yes": true}}
// "yes" parameter removed from schema; ignored if sent
// DenyConfirmer denies; response: {"error": {"code": "E_DENIED", "safer_alternative": "drop --purge"}}
```

### Bad: Bypass without audit (prevented)

```bash
$ filex rm important.txt --purge --i-understand-this-is-destructive
# BypassConfirmer accepts; audit MUST contain bypass=true, confirm_source=bypass
# Missing audit record = bug
```

---

## Tests Required

### Unit Tests

- **Classification**: Table-driven test covering 4 classifications × {interactive, non-interactive} × {enabled, disabled} → assert outcome / class / reason / safer_alternative.
- **Pure function**: `Evaluate` must not read files, env vars, or terminal state. Mock `Environment` and assert deterministic output.
- **Console detection**: `TestTTYConfirmerNeverReadsPipedStdin` — pipe `y` to stdin, assert `ErrNoTerminal`.
- **Confirmer sources**: `TestConfirmerSources` — assert TTY/Deny/Bypass return correct `source` strings.
- **Prompt parsing**: `TestPromptAcceptsOnlyYes` — only `y` / `Y` / padded `y` accepted; `n` / enter / other rejected.

### Integration Tests

- **Pipeline rejection**: `TestProbeGuardPipelineRejected` — pipe `y` to `filex rm --purge`, assert exit 3 + `E_DENIED`.
- **Bypass audit**: `TestProbeBypassIsAudited` — use bypass flag, assert audit record contains `bypass=true`.
- **MCP denial**: `TestProbeMCPDeniesPurge` — MCP tool call with `purge:true`, assert structured deny response.
- **Config disable**: `TestProbeGuardDisabledByConfigRunsPurge` — `guard.enabled=false` in config, assert operation succeeds.
- **Broken config**: `TestProbeBrokenConfigDoesNotDisableGuard` — malformed config, assert guard still fires.

### CI Coverage

All unit tests + pipeline rejection + bypass audit + MCP denial + config tests run in CI. Real-terminal y/N prompt is **manual verification only** (see `docs/VERIFY.md`).

---

## Wrong vs Correct

### Wrong: Fallback to stdin on console failure

```go
// ❌ WRONG
func Confirm(d Decision) (string, error) {
    f, err := console.OpenInput()
    if err != nil {
        f = os.Stdin  // fallback to stdin
    }
    // prompt on f
}
```

**Why it's wrong**: An agent-spawned process with piped stdin can feed `y` programmatically. The whole point of the guard is to require a **real terminal**, not just "some input stream".

### Correct: Hard fail on console absence

```go
// ✅ CORRECT
func Confirm(d Decision) (string, error) {
    f, err := console.OpenInput()
    if err != nil {
        return "none", errs.DeniedByGuard(...)  // no fallback
    }
    defer f.Close()
    // prompt on f
}
```

**Why it's correct**: `console.OpenInput()` checks both device open **and** stdin character-device status. Failure means "no real terminal" → deny immediately.

---

### Wrong: Model self-authorization via tool parameter

```go
// ❌ WRONG (pre-fix)
type RmParams struct {
    Path   string
    Purge  bool
    Yes    bool  // model sets this to true
}

func Rm(p RmParams) error {
    if p.Yes {
        // skip confirmation
    }
}
```

**Why it's wrong**: The model can authorize its own destructive actions by setting `yes:true`. This defeats the purpose of an independent confirmation channel.

### Correct: Remove authorization parameter

```go
// ✅ CORRECT (post-fix)
type RmParams struct {
    Path   string
    Purge  bool
    // Yes field removed
    DryRun bool
}

func Rm(p RmParams, rt Runtime) error {
    // rt.Confirmer is bound by the entry point (CLI/MCP)
    // MCP always uses DenyConfirmer
    decision := guard.Evaluate(...)
    if decision.Outcome == guard.OutcomeDeny {
        return errs.DeniedByGuard(...)
    }
}
```

**Why it's correct**: Authorization comes from the **confirmer bound at the entry point**, not from a model-controlled parameter. MCP entry point binds `DenyConfirmer` → always denies high-risk ops.

---

### Wrong: Bypass without audit

```go
// ❌ WRONG
if bypassFlag {
    // proceed without recording
}
```

**Why it's wrong**: Bypass is a security-sensitive action. Without audit, there's no way to detect abuse or debug "did the breaker fire?".

### Correct: Mandatory bypass audit

```go
// ✅ CORRECT
if bypassFlag {
    if !cfg.AllowBypass {
        return errs.DeniedByGuard(...)
    }
    recordJournal(Entry{
        // ...
        Bypass:        true,
        ConfirmSource: "bypass",
        Result:        "bypassed",
    })
}
```

**Why it's correct**: Every bypass leaves an audit trail with `bypass=true` + `confirm_source=bypass`. Missing audit = bug.

---

## Design Decisions

### Why 4 hardcoded classifications instead of a rule engine?

**Context**: We needed to block high-risk operations with independent confirmation.

**Options considered**:
1. YAML-configured classification rules with pattern matching
2. Hardcoded switch statement with 4 cases
3. Plugin system for custom classifications

**Decision**: Option 2 (hardcoded). The classification set is small (4 items), stable, and security-critical. A rule engine adds configuration surface, testing complexity, and the risk of misconfiguration disabling protections. If the set grows to 6–7 items, revisit with a rule engine (→ backlog B3).

**Extensibility**: Adding a 5th classification means adding a case to the `classify` function and a test to the matrix. No config schema change, no migration.

---

### Why pure-function Evaluate?

**Context**: The guard decision must be testable without mocking file systems, terminals, or environment variables.

**Decision**: `Evaluate(action, env, cfg)` reads no global state. All inputs are explicit parameters. This makes the decision layer 100% unit-testable and eliminates a class of "it works on my machine" bugs.

**Trade-off**: The caller must gather `Environment.Interactive` by attempting `console.OpenInput()`. This pushes I/O to the boundary, where it belongs.

---

### Why `ErrNoTerminal` instead of `os.Stdin` fallback?

**Context**: Early prototype fell back to stdin when console open failed. This allowed agent-spawned processes to pipe `y`.

**Decision**: Hard fail. The guard's purpose is to require a **real human at a real terminal**. Piped stdin is not that. `console.OpenInput()` checks both device open **and** stdin character-device status.

**Security property**: `TestTTYConfirmerNeverReadsPipedStdin` asserts this invariant. If the test ever fails, the guard is broken.

---

## Common Mistakes

### Mistake: Adding a 5th classification without updating the test matrix

**Symptom**: New high-risk operation is not blocked in non-interactive mode.

**Cause**: The 4-classification matrix is hardcoded in `TestGuardedActionsWithoutTerminal`. Adding a case to `classify` without adding a row to the test leaves a gap.

**Fix**: Every new classification must add a row to **both** `TestGuardedActionsAtTerminal` and `TestGuardedActionsWithoutTerminal`.

**Prevention**: Code review checklist item: "Does this change add a classification? If yes, are both test matrices updated?"

---

### Mistake: Binding TTYConfirmer in MCP entry point

**Symptom**: MCP tool calls prompt for confirmation, but stdio is JSON-RPC → hang.

**Cause**: MCP entry point must bind `DenyConfirmer`, not `TTYConfirmer`. stdio is already occupied by JSON-RPC; there's no terminal to prompt.

**Fix**: `internal/mcpserver/server.go` binds `DenyConfirmer` with `Environment.Interactive=false`.

**Prevention**: The MCP entry point and CLI entry point have **different confirmer bindings**. Never share this code.

---

### Mistake: Omitting `safer_alternative` from denial

**Symptom**: Agent receives `E_DENIED` but doesn't know what to do instead.

**Cause**: `errs.DeniedByGuard` requires a `safer_alternative` parameter. Passing an empty string or prose ("consider whether you really want to do this") is not actionable.

**Fix**: Always name a concrete command: "drop `--purge`", "use `--scope user`", "write to a different path".

**Prevention**: `TestSaferAlternativesOfferAWayOut` asserts every classification has a non-empty `safer_alternative`.

---

## Forbidden Patterns

- **Returning `error` from `internal/ops` on guard denial** — must return `*errs.Error` with code `E_DENIED`.
- **Defaulting to allow on console failure** — if terminal detection fails, the answer is **deny**.
- **Accepting `yes` from MCP tool parameters** — the `yes` field was removed from schemas; if a client sends it, ignore (don't error).
- **Bypassing the guard in `cmd/filex` or `internal/mcpserver`** — the guard must fire in `internal/ops`, where all entry points converge.
- **Logging bypass without `bypass=true`** — every bypass must leave an audit trail; missing audit = security bug.
- **Making `Evaluate` read global state** — it must be a pure function of `(Action, Environment, Config)`.

---

## Required Patterns

- **Bind confirmer at entry point** — CLI binds `TTYConfirmer`; MCP binds `DenyConfirmer`.
- **Use `errs.DeniedByGuard` for refusals** — never format a prose message into `Error.Message`.
- **Audit every denial** — a log of only successes cannot answer "did the breaker fire?".
- **Check `console.OpenInput()` for both device and stdin** — opening `CONIN$` is not enough; stdin must be a character device.
- **Make `safer_alternative` actionable** — name a command, not a suggestion.
- **Test both interactive and non-interactive paths** — every classification needs two test rows.

---

## Related

- [Error Handling](./error-handling.md) — `E_DENIED` code and envelope format
- [Logging Guidelines](./logging-guidelines.md) — audit field semantics
- [Quality Guidelines](./quality-guidelines.md) — platform-specific test guards
- `docs/VERIFY.md` — manual verification checklist (real-terminal y/N prompt)
