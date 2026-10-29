package ops

import (
	"filex/internal/backup"
	"filex/internal/errs"
)

type BackupListResult struct {
	Backups []backup.BackupEntry `json:"backups"`
}

func BackupList(filter string) (*BackupListResult, *errs.Error) {
	entries, err := backup.ListBackups(filter)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "list backups: %v", err)
	}
	return &BackupListResult{Backups: entries}, nil
}

type BackupRestoreResult struct {
	Backup backup.BackupEntry `json:"backup"`
	Restored string           `json:"restored"`
}

func BackupRestore(id string) (*BackupRestoreResult, *errs.Error) {
	entry, err := backup.RestoreBackup(id)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "restore backup: %v", err)
	}
	return &BackupRestoreResult{
		Backup:   *entry,
		Restored: entry.Path,
	}, nil
}

type BackupPruneResult struct {
	Removed int `json:"removed"`
}

func BackupPrune(retention int) (*BackupPruneResult, *errs.Error) {
	if retention <= 0 {
		retention = backup.DefaultRetention
	}
	removed, err := backup.PruneBackups(retention)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "prune backups: %v", err)
	}
	return &BackupPruneResult{Removed: removed}, nil
}
