package main

import (
	"os"
	"strconv"
	"strings"

	"filex/internal/errs"
	"filex/internal/ops"
)

func runTail(args []string) *errs.Error {
	var path string
	lines := 10
	follow := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--lines" && i+1 < len(args):
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return errs.Newf(errs.EUsage, "invalid --lines: %s", args[i])
			}
			lines = n
		case strings.HasPrefix(args[i], "--lines="):
			n, err := strconv.Atoi(strings.TrimPrefix(args[i], "--lines="))
			if err != nil {
				return errs.Newf(errs.EUsage, "invalid --lines value")
			}
			lines = n
		case args[i] == "--follow":
			follow = true
		case !strings.HasPrefix(args[i], "-"):
			path = args[i]
		}
	}

	if path == "" {
		return errs.WithHint(errs.New(errs.EUsage, "file path required"), "filex tail [--lines=N] [--follow] <path>")
	}

	result, err := ops.Tail(ops.TailParams{
		Path:   path,
		Lines:  lines,
		Follow: follow,
	})
	if err != nil {
		return err
	}

	if wErr := errs.WriteSuccess(os.Stdout, result); wErr != nil {
		return errs.Newf(errs.EIO, "write output: %v", wErr)
	}
	return nil
}

func runGrep(args []string) *errs.Error {
	var pattern string
	var paths []string
	max := 100
	stream := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--max" && i+1 < len(args):
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return errs.Newf(errs.EUsage, "invalid --max: %s", args[i])
			}
			max = n
		case strings.HasPrefix(args[i], "--max="):
			n, err := strconv.Atoi(strings.TrimPrefix(args[i], "--max="))
			if err != nil {
				return errs.Newf(errs.EUsage, "invalid --max value")
			}
			max = n
		case args[i] == "--stream":
			stream = true
		case args[i] == "--paths" && i+1 < len(args):
			i++
			paths = strings.Split(args[i], ",")
		case strings.HasPrefix(args[i], "--paths="):
			paths = strings.Split(strings.TrimPrefix(args[i], "--paths="), ",")
		case !strings.HasPrefix(args[i], "-"):
			if pattern == "" {
				pattern = args[i]
			} else {
				paths = append(paths, args[i])
			}
		}
	}

	if pattern == "" {
		return errs.WithHint(errs.New(errs.EUsage, "pattern required"), "filex grep [--stream] [--max=N] <pattern> [paths...]")
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}

	result, err := ops.Grep(ops.GrepParams{
		Pattern: pattern,
		Paths:   paths,
		Max:     max,
		Stream:  stream,
	})
	if err != nil {
		return err
	}

	if wErr := errs.WriteSuccess(os.Stdout, result); wErr != nil {
		return errs.Newf(errs.EIO, "write output: %v", wErr)
	}
	return nil
}
