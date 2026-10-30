package ops

import (
	"os"
	"path/filepath"

	"filex/internal/errs"
	"filex/internal/guard"
	"filex/internal/journal"
	"filex/internal/policy"
)

func loadPolicy() policy.Config {
	cfg, _ := policy.Load()
	return cfg
}

func checkPolicy(path string) *errs.Error {
	cfg := loadPolicy()
	result := policy.Check(path, cfg)
	if !result.Allowed {
		return errs.WithHint(
			errs.Newf(errs.EDenied, "policy denied: %s", result.Reason),
			"check that the path is within allowed directories and not in a denied system directory",
		)
	}
	return nil
}

// Runtime is what an entry point binds before touching anything: who is asking,
// whether a human is reachable, and who may ask them. Bound once at process
// start, then read by every operation.
type Runtime struct {
	Actor         string
	SessionID     string
	SessionSource string
	Interactive   bool
	Confirmer     guard.Confirmer
}

// The zero behaviour is to deny: an entry point that forgets to bind gets the
// safest answer rather than an implicit approval.
var rt = Runtime{Actor: "cli", Confirmer: guard.DenyConfirmer{}}

func BindRuntime(r Runtime) {
	if r.Confirmer == nil {
		r.Confirmer = guard.DenyConfirmer{}
	}
	if r.Actor == "" {
		r.Actor = "cli"
	}
	rt = r
}

// ruling is what gate() learned about one operation: the decision, and how it
// was answered.
type ruling struct {
	decision      guard.Decision
	confirmSource string
	bypass        bool
}

// stamp attaches the decision chain to an audit entry. A nil ruling means the
// operation was never guard-relevant, and the entry then stays as it was.
func (r *ruling) stamp(e *journal.Entry) {
	if r == nil {
		return
	}
	e.GuardClass = r.decision.Class
	e.Decision = string(r.decision.Outcome)
	e.DecisionReason = r.decision.Reason
	e.ConfirmSource = r.confirmSource
	e.Bypass = r.bypass
}

// configRoot is ~/.filex. Writing inside it can widen filex's own limits, which
// is why that path is a guarded action rather than an ordinary write.
func configRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".filex")
}

// gate is the single decision point for destructive operations. Order is fixed:
// checkPolicy (already run by the caller) → guard.Evaluate → Confirmer.Confirm.
// It lives here because internal/ops is the one layer both the CLI and MCP
// paths converge on; gating at either entry point would leave the other bypass.
//
// The confirmer is consulted for both require_confirmation and deny. A deny only
// means no human is reachable right now, which a bypass flag answers in advance;
// and with no flag installed the answer is provably still no, since TTYConfirmer
// opens the same console handle that set Interactive=false.
//
// Denied operations are audited too — a log of only what succeeded cannot
// answer what was blocked.
func gate(a guard.Action) (*ruling, *errs.Error) {
	cfg := loadPolicy()

	a.Actor = rt.Actor
	if a.ConfigRoot == "" {
		a.ConfigRoot = configRoot()
	}

	d := guard.Evaluate(a, cfg.Guard, guard.Environment{Interactive: rt.Interactive})
	if d.Outcome == guard.OutcomeAllow {
		return &ruling{decision: d}, nil
	}

	confirmed, source, err := rt.Confirmer.Confirm(a, d)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "confirmation failed: %v", err)
	}
	// The config gets the last word on bypass, so an entry point that installs
	// the confirmer by mistake cannot override it.
	if confirmed && source == guard.SourceBypass && !cfg.Guard.AllowsBypass() {
		confirmed = false
		source = guard.SourceNone
	}
	if !confirmed {
		recordDenied(a, d, source)
		return nil, deniedError(d, denialMessage(d))
	}
	return &ruling{decision: d, confirmSource: source, bypass: source == guard.SourceBypass}, nil
}

// denialMessage says what actually happened: without a terminal the guard made
// the call, with one it was the human who declined.
func denialMessage(d guard.Decision) string {
	if d.Outcome == guard.OutcomeDeny {
		return "denied by guard"
	}
	return "not confirmed"
}

func deniedError(d guard.Decision, message string) *errs.Error {
	return errs.DeniedByGuard(message, d.Class, d.Reason, d.SaferAlternative)
}

// recordExecuted writes the audit record for an operation that ran.
func recordExecuted(op, path, backupID, shaBefore, shaAfter, detail string, r *ruling) {
	e := journal.Entry{
		Op:           op,
		Path:         path,
		BackupID:     backupID,
		SHA256Before: shaBefore,
		SHA256After:  shaAfter,
		Detail:       detail,
		Result:       "executed",
	}
	applyRuntime(&e)
	r.stamp(&e)
	_ = journal.Record(e)
}

// recordEnv audits an environment change. Sensitive values are hashed by
// journal.MaskEnvEntry, never stored as text.
func recordEnv(op, scope, name, value string, r *ruling) {
	e := journal.MaskEnvEntry(op, scope, name, value)
	e.Result = "executed"
	applyRuntime(&e)
	r.stamp(&e)
	_ = journal.Record(e)
}

// recordDenied writes the audit record for an operation that never ran.
func recordDenied(a guard.Action, d guard.Decision, confirmSource string) {
	e := journal.Entry{
		Op:             string(a.Verb),
		Path:           a.Path,
		Scope:          a.Scope,
		EnvName:        a.Name,
		GuardClass:     d.Class,
		Decision:       string(d.Outcome),
		DecisionReason: d.Reason,
		ConfirmSource:  confirmSource,
		Result:         "denied",
	}
	applyRuntime(&e)
	_ = journal.Record(e)
}

func applyRuntime(e *journal.Entry) {
	e.Actor = rt.Actor
	e.SessionID = rt.SessionID
	e.SessionSource = rt.SessionSource
}
