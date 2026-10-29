package main

import (
	"os"
	"strconv"

	"filex/internal/errs"
	"filex/internal/ops"
)

func runRead(args []string) *errs.Error {
	var encoding string
	var maxBytes int64
	var offset, limit int
	var path string

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
			} else if i+1 < len(args) && (len(args[i+1]) == 0 || args[i+1][0] != '-') {
				i++
				value = args[i]
			}

			switch name {
			case "encoding":
				encoding = value
			case "max-bytes":
				maxBytes = int64(atoi(value))
			case "offset":
				offset = atoi(value)
			case "limit":
				limit = atoi(value)
			}
		} else if path == "" {
			path = arg
		}
	}

	if path == "" {
		return errs.New(errs.EUsage, "usage: filex read <path> [--encoding=auto] [--max-bytes=N] [--offset=N] [--limit=N]")
	}

	result, err := ops.Read(ops.ReadParams{
		Path:     path,
		Encoding: encoding,
		MaxBytes: maxBytes,
		Offset:   offset,
		Limit:    limit,
	})
	if err != nil {
		return err
	}

	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "write failed: %v", err)
	}
	return nil
}

func runEdit(args []string) *errs.Error {
	var find, replace, expectSHA, encoding string
	var all, dryRun, backup bool
	var path string

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
			} else if i+1 < len(args) && (len(args[i+1]) == 0 || args[i+1][0] != '-') {
				i++
				value = args[i]
			} else {
				value = "true"
			}

			switch name {
			case "find":
				find = value
			case "replace":
				replace = value
			case "all":
				all = true
			case "dry-run":
				dryRun = true
			case "expect-sha256":
				expectSHA = value
			case "encoding":
				encoding = value
			case "backup":
				backup = true
			}
		} else if path == "" {
			path = arg
		}
	}

	if path == "" {
		return errs.New(errs.EUsage, "usage: filex edit <path> --find=<text> --replace=<text> [--all] [--dry-run]")
	}

	if find == "" {
		return errs.New(errs.EUsage, "--find is required")
	}

	result, err := ops.Edit(ops.EditParams{
		Path:         path,
		Find:         find,
		Replace:      replace,
		All:          all,
		DryRun:       dryRun,
		ExpectSHA256: expectSHA,
		Encoding:     encoding,
		Backup:       backup,
	})
	if err != nil {
		return err
	}

	if err := errs.WriteSuccess(os.Stdout, result); err != nil {
		return errs.Newf(errs.EIO, "write failed: %v", err)
	}
	return nil
}

func index(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
