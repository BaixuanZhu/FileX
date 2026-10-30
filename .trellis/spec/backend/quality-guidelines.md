# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

This document captures the project's quality standards, forbidden patterns, and required conventions for backend code.

---

## Forbidden Patterns

### Don't: Write platform-specific tests without guards

**Problem**:
```go
func TestWindowsPath(t *testing.T) {
    result := policy.Check(`C:\Windows\System32`, cfg)
    // Fails on Linux: filepath.VolumeName returns empty, separators differ
}
```

**Why it's bad**: Tests that use Windows-specific paths, APIs, or environment variables (LOCALAPPDATA, registry) fail on Linux/macOS CI runners, breaking cross-platform builds.

**Instead**:
```go
func TestWindowsPath(t *testing.T) {
    if runtime.GOOS != "windows" {
        t.Skip("Windows-specific path test")
    }
    result := policy.Check(`C:\Windows\System32`, cfg)
    // ...
}
```

### Don't: Write to real machine state in tests

**Problem**:
```go
func TestEnvSet(t *testing.T) {
    winenv.WriteStringValue(winenv.ScopeMachine, "TEST_VAR", "value", winenv.KindSZ)
    // Writes to real HKLM registry on developer machine
}
```

**Why it's bad**: Tests that modify the registry, system environment, or other shared machine state can can corrupt the developer's environment or leak leak into other tests.

**Instead**: Use the `requireIsolatedVM` helper to gate tests that must only run in CI:
```go
func TestMachineEnvWrite(t *testing.T) {
    requireIsolatedVM(t, "writes to real machine registry")
    // Now safe to run in GitHub Actions isolated VM
}
```

---

## Required Patterns

### Convention: Platform-specific test guards

**What**: All tests that use Windows-specific paths, APIs, or environment variables must include a `runtime.GOOS` check at the start.

**Why**: Ensures tests pass on all platforms (Linux, macOS, Windows) where CI runs.

**Example**:
```go
func TestWindowsRegistry(t *testing.T) {
    if runtime.GOOS != "windows" {
        t.Skip("registry operations are Windows-only")
    }
    // Windows-specific test code
}
```

**Related**: See `internal/policy/policy_test.go` and `internal/version/link_test.go` for examples.

### Convention: CI-only test gating

**What**: Tests that write to real machine state (registry, system env, file system outside temp dirs) must use the `requireIsolatedVM` helper.

**Why**: Prevents accidental execution on developer machines while allowing full validation in CI's isolated VMs.

**Example**:
```go
func requireIsolatedVM(t *testing.T, why string) {
    t.Helper()
    if runtime.GOOS != "windows" {
        t.Skip("environment scopes are a Windows concept")
    }
    if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("TEST_ENV_CI") != "1" {
        t.Skipf("needs the isolated CI VM: %s", why)
    }
}

func TestMachineScopeEnv(t *testing.T) {
    requireIsolatedVM(t, "writes to real machine environment")
    // Test code that modifies HKLM
}
```

**Related**: See `cmd/filex/env_gate_integration_test.go` for the canonical implementation.

### Convention: Code formatting

**What**: All Go source files must pass `gofmt -l .` with empty output (no files listed).

**Why**: Ensures consistent formatting across the codebase; enforced by CI's `Gofmt` job.

**How to check locally**:
```bash
gofmt -l .
# If any files are listed, run:
gofmt -w .
```

**CI enforcement**: The `Gofmt` job in `.github/workflows/ci.yml` runs on ubuntu-latest and fails if any files need formatting.

---

## Testing Requirements

### Unit tests

- All public functions must have unit tests
- Tests must be platform-agnostic or properly guarded (see Forbidden Patterns)
- Use table-driven tests for multiple cases
- Target: >80% coverage for critical paths (guard, ops, journal)

### Integration tests

- Tag with `//go:build integration`
- Use temp directories for file system operations
- Use `requireIsolatedVM` for tests that modify real machine state
- Run locally with: `go test -tags=integration ./...`
- Run in CI with: `TEST_ENV_CI=1 go test -tags=integration ./...`

### Cross-platform compatibility

- Tests must pass on both ubuntu-latest and windows-latest
- Use `runtime.GOOS` checks for platform-specific behavior
- Avoid hardcoded paths (use `filepath.Join` with temp dirs)
- Avoid platform-specific environment variables without guards

---

## Code Review Checklist

- [ ] All new tests have platform guards if they use Windows-specific APIs
- [ ] Tests that write to machine state use `requireIsolatedVM`
- [ ] Code passes `gofmt -l .` (no output)
- [ ] Code passes `go vet ./...`
- [ ] Code passes `go vet -tags=integration ./...`
- [ ] Unit tests pass: `go test ./...`
- [ ] Integration tests pass locally (with appropriate skips): `go test -tags=integration ./...`
- [ ] No hardcoded paths or environment variables without guards
- [ ] Error messages are specific and actionable
- [ ] Audit records include all required fields (see logging-guidelines.md)
