package main

import (
	"os"

	"filex/internal/errs"
	"filex/internal/ops"
)

func runStat(args []string) *errs.Error {
	if len(args) < 1 {
		return errs.New(errs.EUsage, "usage: filex stat <path>")
	}
	result, err := ops.Stat(args[0])
	if err != nil {
		return err
	}
	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "write failed: %v", err)
	}
	return nil
}

func runLock(args []string) *errs.Error {
	if len(args) < 1 {
		return errs.New(errs.EUsage, "usage: filex lock <path>")
	}
	result, err := ops.Lock(args[0])
	if err != nil {
		return err
	}
	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "write failed: %v", err)
	}
	return nil
}

func runLs(args []string) *errs.Error {
	path := "."
	depth := 1

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--json" {
			continue
		}
		if len(arg) > 8 && arg[:8] == "--depth=" {
			depth = atoi(arg[8:])
			continue
		}
		if len(arg) == 0 || arg[0] != '-' {
			path = arg
		}
	}

	result, err := ops.Ls(path, depth)
	if err != nil {
		return err
	}
	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "write failed: %v", err)
	}
	return nil
}

func runHash(args []string) *errs.Error {
	if len(args) < 1 {
		return errs.New(errs.EUsage, "usage: filex hash <path>")
	}
	result, err := ops.Hash(args[0])
	if err != nil {
		return err
	}
	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "write failed: %v", err)
	}
	return nil
}

func runWrite(args []string) *errs.Error {
	var path, content, encoding, expectSHA string
	var createParents, backupFlag bool

	// Boolean flags must not consume the next positional argument.
	boolFlags := map[string]bool{
		"create-parents": true,
		"backup":         true,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if len(arg) > 0 && arg[0] == '-' {
			name := arg[1:]
			if len(name) > 0 && name[0] == '-' {
				name = name[1:]
			}
			var value string
			if idx := index(name, '='); idx >= 0 {
				value = name[idx+1:]
				name = name[:idx]
			} else if !boolFlags[name] && i+1 < len(args) && (len(args[i+1]) == 0 || args[i+1][0] != '-') {
				i++
				value = args[i]
			}

			switch name {
			case "encoding":
				encoding = value
			case "create-parents":
				createParents = true
			case "expect-sha256":
				expectSHA = value
			case "backup":
				backupFlag = true
			}
		} else if path == "" {
			path = arg
		} else if content == "" {
			content = arg
		}
	}

	if path == "" {
		return errs.New(errs.EUsage, "usage: filex write <path> <content> [--encoding=utf8] [--create-parents] [--expect-sha256=<hash>] [--backup]")
	}

	result, err := ops.Write(ops.WriteParams{
		Path:          path,
		Content:       content,
		Encoding:      encoding,
		CreateParents: createParents,
		ExpectSHA256:  expectSHA,
		Backup:        backupFlag,
	})
	if err != nil {
		return err
	}
	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "write failed: %v", err)
	}
	return nil
}

func runRm(args []string) *errs.Error {
	var path string
	var purge, dryRun bool

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--purge" {
			purge = true
		} else if arg == "--dry-run" {
			dryRun = true
		} else if arg == "--yes" {
			return errs.WithHint(
				errs.New(errs.EUsage, "--yes no longer exists: it never skipped anything"),
				"run --dry-run to preview; irreversible operations now ask for confirmation on the terminal",
			)
		} else if len(arg) == 0 || arg[0] != '-' {
			path = arg
		}
	}

	if path == "" {
		return errs.New(errs.EUsage, "usage: filex rm <path> [--purge] [--dry-run]")
	}

	result, err := ops.Rm(ops.RmParams{
		Path:   path,
		Purge:  purge,
		DryRun: dryRun,
	})
	if err != nil {
		return err
	}
	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "write failed: %v", err)
	}
	return nil
}

func runBackup(args []string) *errs.Error {
	if len(args) < 1 {
		return errs.New(errs.EUsage, "usage: filex backup <list|restore|prune> [args...]")
	}

	action := args[0]
	switch action {
	case "list":
		filterPath := ""
		for i := 1; i < len(args); i++ {
			if len(args[i]) > 7 && args[i][:7] == "--path=" {
				filterPath = args[i][7:]
			}
		}
		result, listErr := ops.BackupList(filterPath)
		if listErr != nil {
			return listErr
		}
		if err := errs.WriteSuccess(os.Stdout, result.Backups); err != nil {
			return errs.Newf(errs.EIO, "write failed: %v", err)
		}
		return nil

	case "restore":
		if len(args) < 2 {
			return errs.New(errs.EUsage, "usage: filex backup restore <backup-id>")
		}
		result, restoreErr := ops.BackupRestore(args[1])
		if restoreErr != nil {
			return restoreErr
		}
		if err := errs.WriteSuccess(os.Stdout, result.Backup); err != nil {
			return errs.Newf(errs.EIO, "write failed: %v", err)
		}
		return nil

	case "prune":
		retention := 3
		for i := 1; i < len(args); i++ {
			if len(args[i]) > 12 && args[i][:12] == "--retention=" {
				retention = atoi(args[i][12:])
			}
		}
		result, pruneErr := ops.BackupPrune(retention)
		if pruneErr != nil {
			return pruneErr
		}
		if err := errs.WriteSuccess(os.Stdout, map[string]int{"pruned": result.Removed}); err != nil {
			return errs.Newf(errs.EIO, "write failed: %v", err)
		}
		return nil

	default:
		return errs.Newf(errs.EUsage, "unknown backup action: %s", action)
	}
}
