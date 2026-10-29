package ops

import (
	"filex/internal/errs"
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

func recordJournal(op, path, backupID, shaBefore, shaAfter string) {
	_ = journal.Record(journal.Entry{
		Op:           op,
		Path:         path,
		BackupID:     backupID,
		SHA256Before: shaBefore,
		SHA256After:  shaAfter,
	})
}
