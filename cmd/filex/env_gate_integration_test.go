//go:build integration

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// Environment scope probes. They are separate from the rest of the gate probes
// because they can touch real machine state: the assertions hold only while the
// guard refuses before internal/winenv is called, and a regression there writes
// into the registry of whatever machine runs them.

const machineEnvKey = `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`

// requireIsolatedVM skips a test that must not run on a developer machine. The
// build tag is not enough on its own — anyone can type -tags=integration — so the
// rule documented in docs/VERIFY.md needs a check in the test itself.
func requireIsolatedVM(t *testing.T, why string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("environment scopes are a Windows concept")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("TEST_ENV_CI") != "1" {
		t.Skipf("needs the isolated CI VM: %s", why)
	}
}

// regValueExists asks reg.exe for one registry value.
func regValueExists(t *testing.T, key, value string) bool {
	t.Helper()
	out, err := exec.Command("reg", "query", key, "/v", value).CombinedOutput()
	if err == nil {
		return true
	}
	// "unable to find" is how reg.exe reports a genuinely absent value. Any other
	// failure would also land here, and a check that silently reports "absent" is
	// exactly how "the registry was not touched" starts passing on a broken VM.
	if strings.Contains(string(out), "unable to find") {
		return false
	}
	t.Fatalf("reg query %s\\%s: %v\n%s", key, value, err, out)
	return false
}

// The machine-scope denial has to be proven against the registry, not just against
// the error message: the guard sits in front of the elevation check and the
// registry write, so a wrong order still ends in an HKLM change.
func TestProbeMachineScopeEnvDeniedWithoutTerminal(t *testing.T) {
	requireIsolatedVM(t, "a regression here writes to the machine environment, and the CI runner is elevated")

	p := newProbe(t)
	name := fmt.Sprintf("FILEX_PROBE_MACHINE_%d", os.Getpid())

	if regValueExists(t, machineEnvKey, name) {
		t.Fatalf("probe value %q already exists in the machine environment", name)
	}
	// Registered before the assertions: if the guard has regressed, the value is
	// already in HKLM by the time the test fails, and it must not stay there.
	t.Cleanup(func() {
		if _, err := exec.Command("reg", "query", machineEnvKey, "/v", name).CombinedOutput(); err == nil {
			t.Errorf("probe value %q was written into the machine environment, deleting it", name)
			if del, delErr := exec.Command("reg", "delete", machineEnvKey, "/v", name, "/f").CombinedOutput(); delErr != nil {
				t.Errorf("cleanup failed: %v\n%s", delErr, del)
			}
		}
	})

	code, out := p.run("y\nyes\ny\n", "env", "set", name, "injected-by-agent", "--scope=machine")

	if code != 3 {
		t.Fatalf("machine-scope env set exited %d, want 3 (E_DENIED): %s", code, out)
	}
	if !strings.Contains(out, `"code":"E_DENIED"`) || !strings.Contains(out, "machine_env_write") {
		t.Fatalf("want a structured machine_env_write denial, got: %s", out)
	}
	if !strings.Contains(out, "safer_alternative") || !strings.Contains(out, "--scope=user") {
		t.Fatalf("denial did not offer the narrower scope: %s", out)
	}
	if regValueExists(t, machineEnvKey, name) {
		t.Fatal("the denied operation still wrote to HKLM")
	}

	entries := p.entries()
	if len(entries) != 1 {
		t.Fatalf("want one audit record, got %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e["result"] != "denied" || e["decision"] != "deny" {
		t.Fatalf("machine-scope write was not audited as a denial: %+v", e)
	}
	if e["guard_class"] != "machine_env_write" || e["scope"] != "machine" {
		t.Fatalf("record is missing the class or the scope: %+v", e)
	}
	if e["env_name"] != name {
		t.Fatalf("env_name = %v, want %q", e["env_name"], name)
	}
}

// The guard objects to machine scope only. In the same non-interactive process, a
// user-scope write must still work — otherwise the denial is a blanket block on
// env, and nothing distinguishes a precise rule from a broken one.
func TestProbeUserScopeEnvRunsWithoutTerminal(t *testing.T) {
	requireIsolatedVM(t, "it writes to the real user environment")

	p := newProbe(t)
	name := fmt.Sprintf("FILEX_PROBE_USER_%d", os.Getpid())

	code, out := p.run("", "env", "set", name, "probe-value", "--scope=user")
	if code != 0 {
		t.Fatalf("user-scope env set needs approval it should not need: exit %d, %s", code, out)
	}

	t.Cleanup(func() {
		if code, out := p.run("", "env", "unset", name, "--scope=user"); code != 0 {
			t.Errorf("cleanup: could not remove probe value %q (exit %d): %s", name, code, out)
		}
	})

	// Read it back through the product, not through reg.exe: this is the path a
	// later `filex env get` takes.
	code, out = p.run("", "env", "get", name, "--scope=user")
	if code != 0 {
		t.Fatalf("env get after a successful set exited %d: %s", code, out)
	}
	if !strings.Contains(out, `"value":"probe-value"`) || !strings.Contains(out, `"scope":"user"`) {
		t.Fatalf("round trip lost the value or the scope: %s", out)
	}

	entries := p.entries()
	if len(entries) < 1 {
		t.Fatalf("no audit record for the executed write: %+v", entries)
	}
	e := entries[0]
	if e["result"] != "executed" || e["scope"] != "user" || e["env_name"] != name {
		t.Fatalf("unexpected record for the user-scope write: %+v", e)
	}
	// A user-scope write never reaches a guard decision, so the record carries no
	// class at all. This is the field that says "this verb is not guarded".
	if v, ok := e["guard_class"]; ok {
		t.Fatalf("user-scope write was guard-classified as %v: %+v", v, e)
	}
}
