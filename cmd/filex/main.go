package main

import (
	"fmt"
	"os"

	"filex/internal/buildinfo"
	"filex/internal/errs"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		_ = errs.WriteFailure(os.Stdout, errs.New(errs.EUsage, "no verb provided"))
		os.Exit(2)
	}

	verb := os.Args[1]

	var err *errs.Error
	switch verb {
	case "help":
		err = runHelp()
	case "version":
		err = runVersion()
	case "read":
		err = runRead(os.Args[2:])
	case "edit":
		err = runEdit(os.Args[2:])
	case "write":
		err = runWrite(os.Args[2:])
	case "stat":
		err = runStat(os.Args[2:])
	case "lock":
		err = runLock(os.Args[2:])
	case "ls":
		err = runLs(os.Args[2:])
	case "hash":
		err = runHash(os.Args[2:])
	case "rm":
		err = runRm(os.Args[2:])
	case "backup":
		err = runBackup(os.Args[2:])
	case "grep":
		err = runGrep(os.Args[2:])
	case "tail":
		err = runTail(os.Args[2:])
	case "env":
		err = runEnv(os.Args[2:])
	case "mcp":
		err = runMCP(os.Args[2:])
	case "upgrade":
		err = runUpgrade(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown verb: %s\n", verb)
		printUsage()
		_ = errs.WriteFailure(os.Stdout, errs.Newf(errs.EUsage, "unknown verb: %s", verb))
		os.Exit(2)
	}

	if err != nil {
		_ = errs.WriteFailure(os.Stdout, err)
		os.Exit(err.Code.ExitCode())
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: filex <verb> [args...]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Verbs:")
	fmt.Fprintln(os.Stderr, "  help      Show help (use --json for machine-readable)")
	fmt.Fprintln(os.Stderr, "  version   Show version info")
	fmt.Fprintln(os.Stderr, "  read      Read file with encoding detection")
	fmt.Fprintln(os.Stderr, "  write     Atomic file write")
	fmt.Fprintln(os.Stderr, "  edit      Byte-level find/replace")
	fmt.Fprintln(os.Stderr, "  grep      Search with encoding awareness")
	fmt.Fprintln(os.Stderr, "  stat      File metadata")
	fmt.Fprintln(os.Stderr, "  lock      Lock diagnosis")
	fmt.Fprintln(os.Stderr, "  ls        List directory")
	fmt.Fprintln(os.Stderr, "  hash      Compute checksum")
	fmt.Fprintln(os.Stderr, "  rm        Remove (recycle bin by default)")
	fmt.Fprintln(os.Stderr, "  tail      Read last N lines")
	fmt.Fprintln(os.Stderr, "  env       Environment variables (get/set/list/unset)")
	fmt.Fprintln(os.Stderr, "  backup    Backup management (list/restore/prune)")
	fmt.Fprintln(os.Stderr, "  mcp       MCP server (serve/install)")
	fmt.Fprintln(os.Stderr, "  upgrade   Upgrade to latest version")
}

func runVersion() *errs.Error {
	info := buildinfo.Get()
	if err := errs.WriteSuccess(os.Stdout, info); err != nil {
		return errs.Newf(errs.EIO, "failed to write version: %v", err)
	}
	return nil
}
