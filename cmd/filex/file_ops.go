package main

import (
	"os"

	"filex/internal/errs"
	"filex/internal/ops"
	"filex/internal/backup"
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
	var purge, yes bool

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--purge" {
			purge = true
		} else if arg == "--yes" {
			yes = true
		} else if arg == "--dry-run" {
			continue
		} else if len(arg) == 0 || arg[0] != '-' {
			path = arg
		}
	}

	if path == "" {
		return errs.New(errs.EUsage, "usage: filex rm <path> [--purge] [--yes]")
	}

	result, err := ops.Rm(ops.RmParams{
		Path:  path,
		Purge: purge,
		Yes:   yes,
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
		backups, err := backup.ListBackups(filterPath)
		if err != nil {
			return errs.Newf(errs.EIO, "list failed: %v", err)
		}
		if err := errs.WriteSuccess(os.Stdout, backups); err != nil {
			return errs.Newf(errs.EIO, "write failed: %v", err)
		}
		return nil

	case "restore":
		if len(args) < 2 {
			return errs.New(errs.EUsage, "usage: filex backup restore <backup-id>")
		}
		entry, err := backup.RestoreBackup(args[1])
		if err != nil {
			if e, ok := err.(*errs.Error); ok {
				return e
			}
			return errs.Newf(errs.EIO, "restore failed: %v", err)
		}
		if err := errs.WriteSuccess(os.Stdout, entry); err != nil {
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
		pruned, err := backup.PruneBackups(retention)
		if err != nil {
			return errs.Newf(errs.EIO, "prune failed: %v", err)
		}
		if err := errs.WriteSuccess(os.Stdout, map[string]int{"pruned": pruned}); err != nil {
			return errs.Newf(errs.EIO, "write failed: %v", err)
		}
		return nil

	default:
		return errs.Newf(errs.EUsage, "unknown backup action: %s", action)
	}
}
