package main

import (
	"fmt"
	"os"

	"filex/internal/buildinfo"
	"filex/internal/console"
	"filex/internal/errs"
	"filex/internal/guard"
	"filex/internal/journal"
	"filex/internal/ops"
	"filex/internal/version"
)

// bypassFlag is deliberately verbose: adding it should feel like making a
// decision, not like completing a command.
const bypassFlag = "--i-understand-this-is-destructive"

func main() {
	// Check if migration is needed
	needsMigration, oldPath, migrationErr := version.NeedsMigration()
	if migrationErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to check migration status: %v\n", migrationErr)
	} else if needsMigration {
		fmt.Println("Migrating from flat installation to versioned structure...")
		if err := version.MigrateFromFlat(oldPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: migration failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "Please run 'filex upgrade' to install the latest version\n")
			os.Exit(1)
		}
		fmt.Println("Migration complete!")
		fmt.Printf("Old binary at %s has been migrated to versioned structure\n", oldPath)
		fmt.Println("Please restart your terminal or run 'filex upgrade' to get the latest version")
		fmt.Println()
	}

	if len(os.Args) < 2 {
		printUsage()
		_ = errs.WriteFailure(os.Stdout, errs.New(errs.EUsage, "no verb provided"))
		os.Exit(2)
	}

	verb := os.Args[1]
	args := os.Args[2:]

	// Bound before any verb runs: every operation reads who is asking and who may
	// answer for them from this, so binding later would leave a window where the
	// default (deny) applies to something a human did approve.
	bindRuntime(verb, args)

	var err *errs.Error
	switch verb {
	case "help":
		err = runHelp()
	case "version":
		err = runVersion()
	case "read":
		err = runRead(args)
	case "edit":
		err = runEdit(args)
	case "write":
		err = runWrite(args)
	case "stat":
		err = runStat(args)
	case "lock":
		err = runLock(args)
	case "ls":
		err = runLs(args)
	case "hash":
		err = runHash(args)
	case "rm":
		err = runRm(args)
	case "backup":
		err = runBackup(args)
	case "journal":
		err = runJournal(args)
	case "grep":
		err = runGrep(args)
	case "tail":
		err = runTail(args)
	case "env":
		err = runEnv(args)
	case "mcp":
		err = runMCP(args)
	case "upgrade":
		err = runUpgrade(args)
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

func bindRuntime(verb string, args []string) {
	// upgrade --yes predates the guard and stays valid as the audited bypass: the
	// prompt it used to skip read os.Stdin, which is exactly the channel an agent
	// driving filex controls.
	bypass := hasFlag(args, bypassFlag) ||
		(verb == "upgrade" && (hasFlag(args, "--yes") || hasFlag(args, "-y")))

	interactive := console.IsInteractive()

	var confirmer guard.Confirmer
	switch {
	case bypass:
		confirmer = guard.BypassConfirmer{}
	case interactive:
		confirmer = guard.TTYConfirmer{}
	default:
		confirmer = guard.DenyConfirmer{}
	}

	// An orchestrator that wants its operations grouped sets the variable; nobody
	// has to, and an unusable value is dropped rather than repaired.
	sessionID, sessionSource := journal.SessionFromEnv()

	ops.BindRuntime(ops.Runtime{
		Actor:         "cli",
		SessionID:     sessionID,
		SessionSource: sessionSource,
		Interactive:   interactive,
		Confirmer:     confirmer,
	})
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
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
	fmt.Fprintln(os.Stderr, "  journal   Audit log analysis (stats/path)")
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
