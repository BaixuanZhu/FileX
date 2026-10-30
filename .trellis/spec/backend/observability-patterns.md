# Observability Patterns

> Read-time aggregation and observation-only architecture for audit analytics.

---

## Overview

FileX provides **observation-only** analytics over its append-only audit journal. The `filex journal stats` command performs read-time aggregation without any caching, indexing, or write-path side effects. This pattern ensures that observability bugs cannot block file operations.

**Key insight**: The journal is both the write target and the read source. There is no separate analytics store, no background aggregation, no cross-process coordination. Statistics are computed on-demand by scanning the log.

---

## Architecture

```
Write path: ops → journal.append (no stats calls)
Read path:  journal stats → scan → aggregate → render
```

The write path never invokes stats code. The read path never modifies the journal. This separation is a **hard constraint**, not a guideline.

---

## Signatures

### Aggregation Core

```go
// internal/journal/stats.go

type StatsParams struct {
    Since   time.Time  // filter: entries after this time
    Until   time.Time  // filter: entries before this time
    Session string     // filter: specific session_id (empty = all)
    GroupBy string     // "session" | "verb" | "guard_class" | "decision" | "bypass" | "source"
    Top     int        // limit group-by to top N (0 = all)
}

type Report struct {
    Overview      OverviewStats
    BySession     []GroupStats
    ByVerb        []GroupStats
    ByGuardClass  []GroupStats
    ByDecision    []GroupStats
    BypassFreq    BypassStats
    SourceComposition []SourceStats
    SkippedLines  int
    TotalLines    int
}

type OverviewStats struct {
    TotalEntries    int
    UniqueSessions  int
    TimeRange       TimeRange
}

type GroupStats struct {
    Key   string
    Count int
    Pct   float64  // percentage of total
}

type BypassStats struct {
    Count int
    Pct   float64
}

type SourceStats struct {
    Source string  // "mcp_connection" | "env" | "absent"
    Count  int
    Pct    float64
}

func Collect(params StatsParams) (*Report, error)
func Scan(reader io.Reader, filter func(Entry) bool) ([]Entry, int, error)
```

### CLI Command

```go
// cmd/filex/journal_stats.go

func journalStatsCmd(args []string) error
// Parses: --since, --until, --session, --group-by, --top, --json
// Renders: text/tabwriter for human output, encoding/json for --json
```

---

## Contracts

### Request Parameters

| Flag | Type | Default | Constraints |
|---|---|---|---|
| `--since` | duration or RFC3339 | none | Duration is "now minus" (e.g., `48h` = last 48 hours) |
| `--until` | duration or RFC3339 | none | Duration is "now minus" |
| `--session` | string | empty | Exact match on `session_id` |
| `--group-by` | string | empty | One of: `session`, `verb`, `guard_class`, `decision`, `bypass`, `source` |
| `--top` | int | 0 | Limit group-by output to top N entries |
| `--json` | bool | false | Output machine-readable JSON |

### Response Format

**Human output** (`--json=false`):
```
== Overview ==
Total entries: 1234
Unique sessions: 5
Time range: 2026-10-29T10:00:00Z to 2026-10-30T15:30:00Z

== By Verb ==
rm       500  (40.5%)
write    400  (32.4%)
edit     200  (16.2%)
env_set  134  (10.9%)

Skipped lines: 2
```

**JSON output** (`--json=true`):
```json
{
  "overview": {
    "total_entries": 1234,
    "unique_sessions": 5,
    "time_range": {
      "start": "2026-10-29T10:00:00Z",
      "end": "2026-10-30T15:30:00Z"
    }
  },
  "by_verb": [
    {"key": "rm", "count": 500, "pct": 40.5},
    {"key": "write", "count": 400, "pct": 32.4}
  ],
  "skipped_lines": 2,
  "total_lines": 1236
}
```

**Stability contract**: JSON field names are fixed. Empty lists serialize as `[]`, not `null`. Scripts can depend on this shape.

### Error Handling

| Condition | Behavior |
|---|---|
| Journal file does not exist | Return zero-value report, exit 0 |
| Journal directory does not exist | Return zero-value report, exit 0, **do not create directory** |
| Malformed JSON line | Increment `skipped_lines`, continue scanning |
| Truncated final line | Increment `skipped_lines`, continue scanning |
| Invalid `--since` / `--until` format | Exit 2 with usage message |
| Invalid `--group-by` value | Exit 2 with usage message |

**Critical**: Stats errors never block file operations. The command exits 0 even with skipped lines, because a partially readable journal is normal (append-only logs have truncated tails during crashes).

---

## Validation & Error Matrix

| Condition | Outcome |
|---|---|
| `--since=48h` | Filter entries from `now - 48h` to `now` |
| `--since=2026-10-29T10:00:00Z` | Filter entries after exact timestamp |
| `--session=abc123` | Filter entries with `session_id == "abc123"` |
| `--group-by=verb --top=5` | Show top 5 verbs by count |
| `--group-by=source` | Show `session_source` composition (mcp_connection / env / absent) |
| Empty journal | Return all-zero report, exit 0 |
| 10 malformed lines | `skipped_lines=10`, rest aggregated normally |

---

## Good / Base / Bad Cases

### Good: Observation-only aggregation

```go
// ✅ CORRECT
func Collect(params StatsParams) (*Report, error) {
    entries, skipped, err := Scan(journalReader, filterByTime(params))
    if err != nil {
        return nil, err
    }
    return aggregate(entries, params.GroupBy, params.Top), nil
}

// Scan reads the journal; Collect aggregates; neither writes anything
```

**Why it's correct**: The read path is completely separate from the write path. A bug in `Collect` cannot corrupt the journal or block file operations.

### Bad: Write-path stats update

```go
// ❌ WRONG
func recordJournal(entry Entry) {
    journal.Append(entry)
    stats.Increment(entry.Verb)  // write-path side effect
}
```

**Why it's wrong**: Stats code on the write path means stats bugs can block file operations. This violates the observation-only architecture.

---

### Good: Tolerant scanning

```go
// ✅ CORRECT
func Scan(reader io.Reader, filter func(Entry) bool) ([]Entry, int, error) {
    scanner := bufio.NewScanner(reader)
    scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)  // 1 MiB limit
    
    var entries []Entry
    skipped := 0
    
    for scanner.Scan() {
        line := scanner.Bytes()
        var entry Entry
        if err := json.Unmarshal(line, &entry); err != nil {
            skipped++  // malformed line, skip and continue
            continue
        }
        if filter(entry) {
            entries = append(entries, entry)
        }
    }
    
    return entries, skipped, scanner.Err()
}
```

**Why it's correct**: Malformed lines are counted but do not abort the scan. A truncated tail (normal for append-only logs) does not prevent aggregation of valid entries.

### Bad: Strict parsing

```go
// ❌ WRONG
func Scan(reader io.Reader) ([]Entry, error) {
    // ...
    if err := json.Unmarshal(line, &entry); err != nil {
        return nil, fmt.Errorf("malformed line: %w", err)  // abort on first error
    }
    // ...
}
```

**Why it's wrong**: A single corrupted line (e.g., from a crash mid-write) makes the entire stats command fail. This is unacceptable for an observation tool.

---

### Good: Backward-compatible field addition

```go
// ✅ CORRECT
type Entry struct {
    Op        string `json:"op"`
    Path      string `json:"path"`
    SessionID string `json:"session_id,omitempty"`      // new field
    SessionSource string `json:"session_source,omitempty"` // new field
}

// Old entries without these fields unmarshal correctly (empty strings)
// New entries with empty values serialize identically to old entries (omitempty)
```

**Why it's correct**: `omitempty` ensures that entries without session information serialize byte-for-byte identical to pre-feature entries. Old log consumers see no change.

### Bad: Breaking serialization

```go
// ❌ WRONG
type Entry struct {
    Op        string `json:"op"`
    SessionID string `json:"session_id"`  // no omitempty
}

// Old entries now serialize as {"op":"rm","session_id":""} instead of {"op":"rm"}
// Byte-for-byte compatibility broken
```

**Why it's wrong**: Log consumers that do byte-level comparison or hashing will see spurious differences. The `compat_test.go` assertion fails.

---

## Tests Required

### Unit Tests

- **Aggregation correctness**: Fixed JSONL fixture → assert each partition (overview, by-verb, by-session, etc.) matches hand-calculated values.
- **Malformed line handling**: Fixture with 10 valid + 2 malformed lines → assert `skipped_lines=2`, valid entries aggregated.
- **Time window filtering**: Entries at `T-1h`, `T-30m`, `T-10m` → `--since=45m` returns last two only.
- **Empty journal**: No journal file → return zero-value report, exit 0, **do not create directory**.
- **Backward compatibility**: Old-format entry (no `session_id` field) → unmarshal succeeds, `session_id=""`, `session_source=""`.
- **Serialization stability**: New-format entry with empty session fields → serialize byte-for-byte identical to old format.
- **JSON shape stability**: `--json` output → assert field names, empty lists are `[]` not `null`.

### Integration Tests

- **End-to-end scan**: Write 100 entries via `ops` → run `journal stats` → assert counts match.
- **Concurrent read/write**: Writer appending while reader scans → no panic, no corruption (append-only is inherently safe).
- **Large journal**: 10k entries → stats completes in <1s (streaming scan, not full load).

### CLI Tests

- **Flag parsing**: Each combination of `--since`, `--until`, `--session`, `--group-by`, `--top`, `--json` → correct filter/aggregation.
- **Invalid flags**: `--since=invalid` → exit 2 with usage message.
- **Human rendering**: `tabwriter` output → columns aligned, percentages formatted.

---

## Wrong vs Correct

### Wrong: Duration as "after this time"

```go
// ❌ WRONG
if params.Since > 0 {
    sinceTime := time.Now().Add(params.Since)  // "48h" means 48 hours from now
    filter = func(e Entry) bool { return e.Time.After(sinceTime) }
}
```

**Why it's wrong**: `--since=48h` should mean "last 48 hours", not "48 hours from now". The first interpretation filters to recent entries; the second filters to future entries (empty result).

### Correct: Duration as "before now"

```go
// ✅ CORRECT
if params.Since > 0 {
    sinceTime := time.Now().Add(-params.Since)  // "48h" means 48 hours ago
    filter = func(e Entry) bool { return e.Time.After(sinceTime) }
}
```

**Why it's correct**: Negative duration → "now minus 48h" → filters to entries after that time → "last 48 hours".

---

### Wrong: Caching aggregation results

```go
// ❌ WRONG
var cache *Report
var cacheTime time.Time

func Collect(params StatsParams) (*Report, error) {
    if time.Since(cacheTime) < 5*time.Minute {
        return cache, nil  // return stale cache
    }
    // ... compute and cache
}
```

**Why it's wrong**: Cache introduces staleness, requires invalidation logic, and adds complexity. The journal is append-only; scanning it is fast (streaming, no index needed). Cache is premature optimization.

### Correct: Always scan fresh

```go
// ✅ CORRECT
func Collect(params StatsParams) (*Report, error) {
    entries, skipped, err := Scan(journalReader, filterByTime(params))
    if err != nil {
        return nil, err
    }
    return aggregate(entries, params.GroupBy, params.Top), nil
}
```

**Why it's correct**: Fresh scan ensures stats reflect the current journal state. No cache invalidation, no staleness, no extra complexity.

---

### Wrong: Creating journal directory on read

```go
// ❌ WRONG
func Collect(params StatsParams) (*Report, error) {
    journalPath := journal.Path()
    if _, err := os.Stat(journalPath); os.IsNotExist(err) {
        os.MkdirAll(filepath.Dir(journalPath), 0755)  // create directory
        return &Report{}, nil
    }
    // ...
}
```

**Why it's wrong**: A read operation should not create directories. If the journal doesn't exist, it means no operations have been performed yet — that's a valid state, not an error to "fix" by creating directories.

### Correct: Return empty report without side effects

```go
// ✅ CORRECT
func Collect(params StatsParams) (*Report, error) {
    journalPath := journal.Path()
    if _, err := os.Stat(journalPath); os.IsNotExist(err) {
        return &Report{}, nil  // no journal, no entries, return empty
    }
    // ... scan existing journal
}
```

**Why it's correct**: Missing journal is a normal state (fresh install, no operations yet). Return empty report, exit 0, no side effects.

---

## Design Decisions

### Why observation-only instead of real-time stats?

**Context**: We needed analytics over the audit journal to answer "is there a problem with agents making too many high-risk operations?"

**Options considered**:
1. Real-time stats: increment counters on write path, maintain in-memory state
2. Background aggregation: periodic goroutine scans journal and updates summary
3. Read-time aggregation: scan journal on-demand when `journal stats` is invoked

**Decision**: Option 3 (read-time). The journal is append-only and typically small (<10k entries). Streaming scan is fast enough (<1s for 10k entries). Real-time stats add write-path complexity and risk (stats bugs block operations). Background aggregation requires a daemon process, which FileX deliberately avoids.

**Trade-off**: Read-time aggregation means stats are not "live" — they reflect the journal state at the time of the query, not real-time. This is acceptable for observation (we're answering "what happened?", not "what's happening right now?").

**Extensibility**: If journals grow to 100k+ entries and scan time becomes unacceptable, introduce a materialized view (separate summary file updated by a background process). But do not do this preemptively — YAGNI.

---

### Why no caching?

**Context**: Repeated `journal stats` calls might benefit from caching.

**Decision**: No cache. The journal is append-only, so cache invalidation is "always invalidate". This makes the cache useless (every call is a fresh scan anyway). Adding a TTL-based cache introduces staleness without reducing scan cost (since the journal changes between calls).

**Simplicity**: No cache = no invalidation logic = no staleness bugs = no configuration (what TTL?). The code is simpler and the behavior is predictable.

---

### Why `omitempty` for session fields?

**Context**: Adding `session_id` and `session_source` to `journal.Entry` must not break backward compatibility.

**Decision**: Both fields use `omitempty`. Entries without session information serialize identically to pre-feature entries. Old log consumers (scripts, parsers) see no change.

**Test**: `internal/journal/compat_test.go` asserts byte-for-byte serialization stability. If this test fails, the change breaks backward compatibility.

**Trade-off**: `omitempty` means "absent" is represented by field omission, not by an explicit `"absent"` value. Readers must normalize empty → absent. This is a small cost for backward compatibility.

---

## Common Mistakes

### Mistake: Writing to journal from stats code

**Symptom**: `journal stats` command creates or modifies journal file.

**Cause**: Stats code accidentally calls `journal.Append` or `journal.Record`.

**Fix**: Stats code must be read-only. Grep for `journal.Append` in `internal/journal/stats.go` and `cmd/filex/journal_stats.go` — should find zero matches.

**Prevention**: Code review checklist: "Does this change add any write calls to stats code?"

---

### Mistake: Aborting scan on malformed line

**Symptom**: `journal stats` fails with "malformed line" error, even though most of the journal is valid.

**Cause**: Scan logic returns error on first malformed line instead of incrementing `skipped_lines` and continuing.

**Fix**: Malformed lines are normal (crash mid-write, truncated tail). Count them, skip them, continue scanning.

**Prevention**: Test fixture must include malformed lines. Assert `skipped_lines > 0` and valid entries aggregated.

---

### Mistake: Creating journal directory on read

**Symptom**: Running `journal stats` on a fresh install creates `~/.filex/journal/` directory.

**Cause**: Stats code calls `os.MkdirAll` to "ensure journal directory exists".

**Fix**: Read operations must not create directories. If journal doesn't exist, return empty report.

**Prevention**: Test: delete journal directory, run `journal stats`, assert directory not recreated.

---

## Forbidden Patterns

- **Calling `journal.Append` from stats code** — stats must be read-only.
- **Caching aggregation results** — journal changes between calls; cache is always stale.
- **Creating journal directory on read** — read operations have no side effects.
- **Aborting scan on malformed line** — truncated tails are normal; skip and continue.
- **Returning error for missing journal** — no journal = no entries = empty report, exit 0.
- **Blocking write path on stats failure** — stats bugs must not affect file operations.
- **Deriving session ID from `ppid` or start time** — unreliable and misattributes operators.

---

## Required Patterns

- **Stream scan with `bufio.Scanner`** — do not load entire journal into memory.
- **1 MiB line buffer limit** — prevent memory exhaustion from malformed lines.
- **Count skipped lines** — observability into journal health.
- **Exit 0 on partial success** — malformed lines are normal, not errors.
- **`omitempty` for new fields** — preserve backward compatibility.
- **Test with malformed fixtures** — assert graceful degradation.
- **Separate read and write paths** — stats code never touches write path.

---

## Related

- [Logging Guidelines](./logging-guidelines.md) — journal schema, session identity fields
- [Guard Patterns](./guard-patterns.md) — guard_class and decision fields aggregated by stats
- `internal/journal/stats.go` — aggregation implementation
- `cmd/filex/journal_stats.go` — CLI command
- `docs/VERIFY.md` — manual verification checklist
