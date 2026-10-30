package guard

import (
	"path/filepath"
	"strings"
	"testing"

	"filex/internal/policy"
)

func boolPtr(b bool) *bool { return &b }

// The zero value is what a config without a guard block deserialises to, so
// these tests also pin down that the guard defaults to on.
var (
	guardOn  = policy.Guard{}
	guardOff = policy.Guard{Enabled: boolPtr(false)}
)

type guardCase struct {
	name       string
	action     Action
	wantClass  string
	wantAltHas string // substring the safer alternative must offer
}

// Paths are built under a real temp directory because the config-root check
// rejects anything that is not absolute — a hand-written "\home\.filex" has no
// drive letter on Windows and would not be a fair test.
func guardedCases(root string) []guardCase {
	config := filepath.Join(root, ".filex")
	work := filepath.Join(root, "work", "src")
	return []guardCase{
		{
			name:       "purge delete",
			action:     Action{Verb: VerbRm, Path: filepath.Join(work, "a.txt"), Purge: true},
			wantClass:  ClassPurgeDelete,
			wantAltHas: "trash",
		},
		{
			name:       "machine env set",
			action:     Action{Verb: VerbEnvSet, Scope: "Machine", Name: "PATH"},
			wantClass:  ClassMachineEnv,
			wantAltHas: "user",
		},
		{
			name:       "machine env unset",
			action:     Action{Verb: VerbEnvUnset, Scope: "Machine", Name: "PATH"},
			wantClass:  ClassMachineEnv,
			wantAltHas: "user",
		},
		{
			name:       "write into own config",
			action:     Action{Verb: VerbWrite, Path: filepath.Join(config, "config.json"), ConfigRoot: config},
			wantClass:  ClassSelfConfig,
			wantAltHas: "hand",
		},
		{
			name:       "edit into own config",
			action:     Action{Verb: VerbEdit, Path: filepath.Join(config, "sub", "notes.txt"), ConfigRoot: config},
			wantClass:  ClassSelfConfig,
			wantAltHas: "hand",
		},
		{
			name:       "self update",
			action:     Action{Verb: VerbUpgrade},
			wantClass:  ClassSelfUpdate,
			wantAltHas: "--check",
		},
	}
}

func TestGuardedActionsAtTerminal(t *testing.T) {
	for _, tc := range guardedCases(t.TempDir()) {
		t.Run(tc.name, func(t *testing.T) {
			d := Evaluate(tc.action, guardOn, Environment{Interactive: true})
			if d.Outcome != OutcomeNeedsConfirm {
				t.Fatalf("outcome = %s, want %s", d.Outcome, OutcomeNeedsConfirm)
			}
			if d.Class != tc.wantClass {
				t.Fatalf("class = %q, want %q", d.Class, tc.wantClass)
			}
			// A human decides from this text alone, so nothing may be blank.
			if d.Reason == "" {
				t.Fatal("reason is empty")
			}
			if d.SaferAlternative == "" {
				t.Fatal("safer alternative is empty")
			}
		})
	}
}

func TestGuardedActionsWithoutTerminal(t *testing.T) {
	for _, tc := range guardedCases(t.TempDir()) {
		t.Run(tc.name, func(t *testing.T) {
			d := Evaluate(tc.action, guardOn, Environment{Interactive: false})
			if d.Outcome != OutcomeDeny {
				t.Fatalf("outcome = %s, want %s", d.Outcome, OutcomeDeny)
			}
			if d.Class != tc.wantClass {
				t.Fatalf("class = %q, want %q", d.Class, tc.wantClass)
			}
			if d.Reason == "" || d.SaferAlternative == "" {
				t.Fatalf("denial must stay actionable: %+v", d)
			}
		})
	}
}

// Each class must name a concrete way out, not just refuse.
func TestSaferAlternativesOfferAWayOut(t *testing.T) {
	for _, tc := range guardedCases(t.TempDir()) {
		t.Run(tc.name, func(t *testing.T) {
			d := Evaluate(tc.action, guardOn, Environment{Interactive: false})
			if !strings.Contains(d.SaferAlternative, tc.wantAltHas) {
				t.Fatalf("alternative %q does not mention %q", d.SaferAlternative, tc.wantAltHas)
			}
		})
	}
}

// Ordinary work must not be interrupted. A gate that cries wolf gets bypassed
// by whoever uses the tool next.
func TestOrdinaryActionsAreAllowed(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, ".filex")
	work := filepath.Join(root, "work", "src")
	cases := []struct {
		name   string
		action Action
	}{
		{"recoverable delete", Action{Verb: VerbRm, Path: filepath.Join(work, "a.txt")}},
		{"write outside config", Action{Verb: VerbWrite, Path: filepath.Join(work, "b.txt"), ConfigRoot: config}},
		{"edit outside config", Action{Verb: VerbEdit, Path: filepath.Join(work, "c.txt"), ConfigRoot: config}},
		{"user env set", Action{Verb: VerbEnvSet, Scope: "User", Name: "MY_TOOL"}},
		{"process env set", Action{Verb: VerbEnvSet, Scope: "Process", Name: "MY_TOOL"}},
		{"user env unset", Action{Verb: VerbEnvUnset, Scope: "User", Name: "MY_TOOL"}},
		{"read", Action{Verb: Verb("read"), Path: filepath.Join(work, "d.txt")}},
		// With no config root resolved, a write is not accused of self-modification
		// on an assumption.
		{"write with unknown config root", Action{Verb: VerbWrite, Path: filepath.Join(work, "e.txt")}},
	}
	for _, tc := range cases {
		for _, interactive := range []bool{true, false} {
			d := Evaluate(tc.action, guardOn, Environment{Interactive: interactive})
			if d.Outcome != OutcomeAllow {
				t.Errorf("%s (interactive=%v): outcome = %s, want allow", tc.name, interactive, d.Outcome)
			}
			if d.Class != "" {
				t.Errorf("%s (interactive=%v): class = %q, want none", tc.name, interactive, d.Class)
			}
		}
	}
}

func TestGuardDisabledAllowsEverything(t *testing.T) {
	for _, tc := range guardedCases(t.TempDir()) {
		for _, interactive := range []bool{true, false} {
			d := Evaluate(tc.action, guardOff, Environment{Interactive: interactive})
			if d.Outcome != OutcomeAllow {
				t.Errorf("%s (interactive=%v): outcome = %s, want allow", tc.name, interactive, d.Outcome)
			}
			// The audit trail has to distinguish "the human switched this off"
			// from "this was never dangerous".
			if d.Class != "disabled" {
				t.Errorf("%s (interactive=%v): class = %q, want disabled", tc.name, interactive, d.Class)
			}
		}
	}
}

// Matching against the config root is anchored: a directory whose name merely
// starts with the config root is not inside it.
func TestConfigRootMatchingIsAnchored(t *testing.T) {
	config := filepath.Join(t.TempDir(), ".filex")
	inside := filepath.Join(config, "sub", "config.json")
	sibling := config + ".bak"
	elsewhere := filepath.Join(t.TempDir(), "other", "filex", "config.json")

	if !withinConfigRoot(inside, config) {
		t.Error("nested file inside the config root not detected")
	}
	if withinConfigRoot(sibling, config) {
		t.Errorf("%q is a sibling of %q, not inside it", sibling, config)
	}
	if withinConfigRoot(elsewhere, config) {
		t.Error("unrelated path matched the config root")
	}
	if withinConfigRoot("", config) || withinConfigRoot(inside, "") {
		t.Error("empty inputs must never match")
	}
	if withinConfigRoot("relative"+string(filepath.Separator)+"config.json", config) {
		t.Error("relative path must not match an absolute root")
	}
}

// Scope arrives from the command line or the registry in whatever casing, so
// the class may not depend on one spelling.
func TestMachineScopeIsCaseInsensitive(t *testing.T) {
	for _, scope := range []string{"machine", "Machine", "MACHINE"} {
		d := Evaluate(Action{Verb: VerbEnvSet, Scope: scope, Name: "PATH"}, guardOn, Environment{Interactive: false})
		if d.Class != ClassMachineEnv {
			t.Errorf("scope %q: class = %q, want %q", scope, d.Class, ClassMachineEnv)
		}
	}
}
