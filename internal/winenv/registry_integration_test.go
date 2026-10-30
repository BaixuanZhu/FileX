//go:build integration && windows

package winenv_test

import (
	"fmt"
	"os"
	"testing"

	"filex/internal/winenv"
)

// TestMain pins this file to the isolated CI VM. Every test below changes the real
// registry of whatever machine runs it, and -tags=integration is a flag a
// developer can type, so the documented rule (docs/VERIFY.md: never run these
// locally) needs a check here rather than only in prose.
func TestMain(m *testing.M) {
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("TEST_ENV_CI") != "1" {
		fmt.Println("skipping registry integration tests: needs TEST_ENV_CI=1 on GitHub Actions, these write the real registry")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestReadWriteUserScope(t *testing.T) {
	name := fmt.Sprintf("FILEX_TEST_%d", os.Getpid())
	defer func() { _ = winenv.DeleteValue(winenv.ScopeUser, name) }()

	if err := winenv.WriteStringValue(winenv.ScopeUser, name, "hello", winenv.KindSZ); err != nil {
		t.Fatalf("write: %v", err)
	}

	val, kind, err := winenv.ReadStringValue(winenv.ScopeUser, name)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if val != "hello" {
		t.Errorf("value = %q, want %q", val, "hello")
	}
	if kind != winenv.KindSZ {
		t.Errorf("kind = %v, want REG_SZ", kind)
	}
}

func TestReadWriteExpandSZ(t *testing.T) {
	name := fmt.Sprintf("FILEX_TEST_EXP_%d", os.Getpid())
	defer func() { _ = winenv.DeleteValue(winenv.ScopeUser, name) }()

	if err := winenv.WriteStringValue(winenv.ScopeUser, name, "%USERPROFILE%\\bin", winenv.KindExpandSZ); err != nil {
		t.Fatalf("write: %v", err)
	}

	val, kind, err := winenv.ReadStringValue(winenv.ScopeUser, name)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if val != "%USERPROFILE%\\bin" {
		t.Errorf("raw value = %q, want %%USERPROFILE%%\\bin", val)
	}
	if kind != winenv.KindExpandSZ {
		t.Errorf("kind = %v, want REG_EXPAND_SZ", kind)
	}
}

func TestDeleteValue(t *testing.T) {
	name := fmt.Sprintf("FILEX_TEST_DEL_%d", os.Getpid())

	if err := winenv.WriteStringValue(winenv.ScopeUser, name, "temp", winenv.KindSZ); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := winenv.DeleteValue(winenv.ScopeUser, name); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, _, err := winenv.ReadStringValue(winenv.ScopeUser, name)
	if err == nil {
		t.Error("expected error reading deleted variable, got nil")
	}
}

func TestReadAllStringValues(t *testing.T) {
	vals, kinds, err := winenv.ReadAllStringValues(winenv.ScopeUser)
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if len(vals) == 0 {
		t.Error("expected at least one user env var")
	}
	if len(vals) != len(kinds) {
		t.Errorf("vals len %d != kinds len %d", len(vals), len(kinds))
	}

	if path, ok := vals["PATH"]; ok {
		t.Logf("PATH = %s", path)
	}
}

func TestBroadcast(t *testing.T) {
	if err := winenv.NotifyEnvironmentChange(); err != nil {
		t.Errorf("broadcast: %v", err)
	}
}
