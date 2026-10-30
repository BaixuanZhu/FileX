package ops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filex/internal/errs"
	"filex/internal/guard"
	"filex/internal/policy"
	"filex/internal/winenv"
)

// writeConfig puts a guard block in the isolated HOME so a test can state which
// policy it is exercising.
func writeConfig(t *testing.T, root, raw string) {
	t.Helper()
	dir := filepath.Join(root, ".filex")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func journalEntries(t *testing.T) []journalEntryView {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".filex", "journal", "filex.jsonl"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var out []journalEntryView
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e journalEntryView
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("decode journal line %q: %v", line, err)
		}
		out = append(out, e)
	}
	return out
}

// journalEntryView is the wire shape of an audit record, read as raw JSON so a
// test fails if a field is renamed rather than silently reading a zero value.
type journalEntryView struct {
	Op             string `json:"op"`
	Path           string `json:"path"`
	Scope          string `json:"scope"`
	EnvName        string `json:"env_name"`
	Detail         string `json:"detail"`
	GuardClass     string `json:"guard_class"`
	Decision       string `json:"decision"`
	DecisionReason string `json:"decision_reason"`
	ConfirmSource  string `json:"confirm_source"`
	Bypass         bool   `json:"bypass"`
	Result         string `json:"result"`
	Actor          string `json:"actor"`
}

func TestBypassIsAudited(t *testing.T) {
	root := isolate(t)
	bindRuntime(t, Runtime{Actor: "test", Confirmer: guard.BypassConfirmer{}})
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	if _, rmErr := Rm(RmParams{Path: path, Purge: true}); rmErr != nil {
		t.Fatalf("bypassed purge: %v", rmErr)
	}

	entries := journalEntries(t)
	if len(entries) != 1 {
		t.Fatalf("journal has %d entries, want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if !e.Bypass {
		t.Fatalf("a bypass without bypass=true cannot be found in an audit later: %+v", e)
	}
	if e.ConfirmSource != guard.SourceBypass {
		t.Fatalf("confirm_source = %q, want %q", e.ConfirmSource, guard.SourceBypass)
	}
	if e.GuardClass != guard.ClassPurgeDelete {
		t.Fatalf("guard_class = %q, want %q", e.GuardClass, guard.ClassPurgeDelete)
	}
	// The guard still recorded its own objection even though the flag overrode it.
	if e.DecisionReason == "" {
		t.Fatalf("decision_reason is empty: %+v", e)
	}
	if e.Result != "executed" {
		t.Fatalf("result = %q, want executed", e.Result)
	}
}

// allow_bypass=false must make the flag inert. The check lives inside gate, not
// in the entry point, so a new entry point cannot forget it.
func TestBypassFlagInertWhenDisallowed(t *testing.T) {
	root := isolate(t)
	writeConfig(t, root, `{"guard":{"allow_bypass":false}}`)
	bindRuntime(t, Runtime{Actor: "test", Confirmer: guard.BypassConfirmer{}})
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	if _, rmErr := Rm(RmParams{Path: path, Purge: true}); rmErr == nil {
		t.Fatal("purge ran though allow_bypass is false")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("inert bypass still deleted the file: %v", err)
	}
	entries := journalEntries(t)
	if len(entries) != 1 || entries[0].Result != "denied" {
		t.Fatalf("want one denied record, got %+v", entries)
	}
	if entries[0].Bypass {
		t.Fatalf("refused bypass must not be recorded as bypass: %+v", entries[0])
	}
}

func TestTTYConfirmationIsAudited(t *testing.T) {
	root := isolate(t)
	h := &humanAtTerminal{confirmed: true}
	bindRuntime(t, Runtime{Actor: "test", Interactive: true, Confirmer: h})
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	if _, rmErr := Rm(RmParams{Path: path, Purge: true}); rmErr != nil {
		t.Fatalf("confirmed purge: %v", rmErr)
	}
	entries := journalEntries(t)
	if len(entries) != 1 {
		t.Fatalf("journal has %d entries, want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.ConfirmSource != guard.SourceTTY {
		t.Fatalf("confirm_source = %q, want %q", e.ConfirmSource, guard.SourceTTY)
	}
	if e.Bypass {
		t.Fatalf("a human at a terminal is not a bypass: %+v", e)
	}
	if e.Decision != string(guard.OutcomeNeedsConfirm) {
		t.Fatalf("decision = %q, want %q", e.Decision, guard.OutcomeNeedsConfirm)
	}
}

func TestDeniedPurgeIsAudited(t *testing.T) {
	root := isolate(t)
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	_, rmErr := Rm(RmParams{Path: path, Purge: true})
	if rmErr == nil {
		t.Fatal("purge succeeded with no confirmation")
	}

	entries := journalEntries(t)
	if len(entries) != 1 {
		t.Fatalf("journal has %d entries, want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Result != "denied" || e.Op != string(guard.VerbRm) {
		t.Fatalf("want a denied rm record, got %+v", e)
	}
	if e.Decision != string(guard.OutcomeDeny) {
		t.Fatalf("decision = %q, want %q", e.Decision, guard.OutcomeDeny)
	}
	if e.GuardClass != guard.ClassPurgeDelete || e.DecisionReason == "" {
		t.Fatalf("denial record is not actionable: %+v", e)
	}
	if e.Path != path {
		t.Fatalf("path = %q, want %q", e.Path, path)
	}
}

// An operator who switched the guard off must not be interrupted by prompts;
// the audit then says why nothing was asked.
func TestGuardDisabledRunsPurge(t *testing.T) {
	root := isolate(t)
	writeConfig(t, root, `{"guard":{"enabled":false}}`)
	bindRuntime(t, Runtime{Actor: "test", Confirmer: guard.DenyConfirmer{}})
	path := filepath.Join(root, "victim.txt")
	writeFile(t, path, "hello")

	if _, rmErr := Rm(RmParams{Path: path, Purge: true}); rmErr != nil {
		t.Fatalf("purge with guard disabled: %v", rmErr)
	}
	entries := journalEntries(t)
	if len(entries) != 1 {
		t.Fatalf("journal has %d entries, want 1: %+v", len(entries), entries)
	}
	if entries[0].GuardClass != "disabled" {
		t.Fatalf("guard_class = %q, want disabled", entries[0].GuardClass)
	}
	if entries[0].Result != "executed" {
		t.Fatalf("result = %q, want executed", entries[0].Result)
	}
}

func TestDefaultPolicyKeepsGuardOn(t *testing.T) {
	var g policy.Guard
	if !g.IsEnabled() {
		t.Fatal("a config with no guard block must leave the guard on")
	}
	if !g.AllowsBypass() {
		t.Fatal("a config with no guard block must allow the bypass flag")
	}
	// Only one field set must not switch the other off.
	partial := `{"guard":{"allow_bypass":false}}`
	var cfg policy.Config
	if err := json.Unmarshal([]byte(partial), &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !cfg.Guard.IsEnabled() {
		t.Fatal(`{"guard":{"allow_bypass":false}} silently disabled the whole guard`)
	}
	if cfg.Guard.AllowsBypass() {
		t.Fatal("allow_bypass=false did not take effect")
	}
}

func TestActorIsStampedOnEveryRecord(t *testing.T) {
	root := isolate(t)
	bindRuntime(t, Runtime{Actor: "mcp:test-client", Confirmer: guard.DenyConfirmer{}})
	path := filepath.Join(root, "keep.txt")
	writeFile(t, path, "hello")

	if _, rmErr := Rm(RmParams{Path: path}); rmErr != nil {
		t.Fatalf("rm: %v", rmErr)
	}
	entries := journalEntries(t)
	if len(entries) != 1 {
		t.Fatalf("journal has %d entries, want 1: %+v", len(entries), entries)
	}
	if entries[0].Actor != "mcp:test-client" {
		t.Fatalf("actor = %q, want mcp:test-client", entries[0].Actor)
	}
}

// Replacing filex's own binary used to be authorised by piping "y" into the
// upgrade prompt. The door the CLI uses must therefore refuse when there is no
// approval to be had.
func TestGateUpgradeDeniedWithoutApproval(t *testing.T) {
	isolate(t)

	commit, gErr := GateUpgrade()
	if gErr == nil {
		t.Fatal("an upgrade was approved with nobody confirming it")
	}
	if commit != nil {
		t.Fatal("a refused gate still handed back a recorder")
	}
	if gErr.GuardClass != guard.ClassSelfUpdate {
		t.Fatalf("class = %q, want %q", gErr.GuardClass, guard.ClassSelfUpdate)
	}
	if gErr.SaferAlternative == "" {
		t.Fatal("the refusal offers no way forward")
	}

	entries := journalEntries(t)
	if len(entries) != 1 || entries[0].Result != "denied" || entries[0].Op != string(guard.VerbUpgrade) {
		t.Fatalf("want one denied upgrade record, got %+v", entries)
	}
}

func TestGateUpgradeWithBypassIsAudited(t *testing.T) {
	root := isolate(t)
	bindRuntime(t, Runtime{Actor: "test", Confirmer: guard.BypassConfirmer{}})

	commit, gErr := GateUpgrade()
	if gErr != nil {
		t.Fatalf("bypassed upgrade gate: %v", gErr)
	}

	// Nothing happens until the caller records what it changed, so an upgrade that
	// failed mid-way leaves no false success in the log.
	if _, err := os.Stat(filepath.Join(root, ".filex", "journal", "filex.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("the gate audited before the upgrade ran: %v", err)
	}

	commit("upgrade", "0.2.0 -> 0.3.0")

	entries := journalEntries(t)
	if len(entries) != 1 {
		t.Fatalf("journal has %d entries, want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Op != "upgrade" || e.Result != "executed" || !e.Bypass {
		t.Fatalf("want an executed bypassed upgrade, got %+v", e)
	}
	if e.Detail != "0.2.0 -> 0.3.0" {
		t.Fatalf("detail = %q, want the version change", e.Detail)
	}
}

func TestGateUpgradeAllowedAtTerminal(t *testing.T) {
	isolate(t)
	h := &humanAtTerminal{confirmed: true}
	bindRuntime(t, Runtime{Actor: "test", Interactive: true, Confirmer: h})

	commit, gErr := GateUpgrade()
	if gErr != nil {
		t.Fatalf("a confirmed upgrade was refused: %v", gErr)
	}
	if len(h.asked) != 1 || h.asked[0].Verb != guard.VerbUpgrade {
		t.Fatalf("upgrade gate asked about %+v", h.asked)
	}
	commit("version_switch", "0.3.0 -> 0.2.0")

	entries := journalEntries(t)
	if len(entries) != 1 || entries[0].ConfirmSource != guard.SourceTTY || entries[0].Bypass {
		t.Fatalf("want a tty-approved record, got %+v", entries)
	}
}

// The machine-scope denial is the one guarded action whose audit record has to
// name the scope and the variable: an operator reading the log should see what
// was refused without opening the registry. EnvSet refuses before internal/winenv
// is reached, so this runs on every platform without touching the registry.
func TestDeniedMachineEnvWriteIsAudited(t *testing.T) {
	isolate(t)
	bindRuntime(t, Runtime{Actor: "test", Confirmer: guard.DenyConfirmer{}})

	_, err := EnvSet(EnvSetParams{
		Scope: winenv.ScopeMachine,
		Name:  "FILEX_TEST_REFUSED",
		Value: "x",
	})
	if err == nil {
		t.Fatal("machine-scope env write was allowed without a terminal")
	}
	if err.Code != errs.EDenied || err.GuardClass != guard.ClassMachineEnv {
		t.Fatalf("denial = %+v, want E_DENIED with class %s", err, guard.ClassMachineEnv)
	}
	if !strings.Contains(err.SaferAlternative, "--scope=user") {
		t.Fatalf("denial did not name the narrower scope: %q", err.SaferAlternative)
	}

	entries := journalEntries(t)
	if len(entries) != 1 {
		t.Fatalf("want one audit record, got %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Op != "env_set" || e.Result != "denied" || e.Decision != "deny" {
		t.Fatalf("record is not a denied env_set: %+v", e)
	}
	if e.Scope != "machine" || e.EnvName != "FILEX_TEST_REFUSED" {
		t.Fatalf("record does not name the scope and the variable: %+v", e)
	}
	if e.GuardClass != guard.ClassMachineEnv || e.ConfirmSource != guard.SourceNone {
		t.Fatalf("class or source missing: %+v", e)
	}
}
