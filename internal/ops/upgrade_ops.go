package ops

import (
	"filex/internal/errs"
	"filex/internal/guard"
)

// GateUpgrade consults the guard about replacing or re-pointing filex's own
// binary. The upgrade flow itself lives in cmd/filex because it is the only
// entry point that exposes it, so this is the door onto gate() for it.
//
// On approval it returns a function that writes the audit record for what
// actually changed. Returning the recorder instead of taking a callback keeps
// the caller's error handling ordinary: a failed upgrade simply never records a
// success.
func GateUpgrade() (func(op, detail string), *errs.Error) {
	r, gErr := gate(guard.Action{Verb: guard.VerbUpgrade})
	if gErr != nil {
		return nil, gErr
	}
	return func(op, detail string) {
		recordExecuted(op, "", "", "", "", detail, r)
	}, nil
}
