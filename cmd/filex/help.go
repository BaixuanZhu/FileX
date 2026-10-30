package main

import (
	"os"

	"filex/internal/errs"
)

type Command struct {
	Verb        string   `json:"verb"`
	Description string   `json:"description"`
	Args        []string `json:"args,omitempty"`
	Flags       []Flag   `json:"flags,omitempty"`
	Stage       string   `json:"stage"`
}

type Flag struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func getCommands() []Command {
	return []Command{
		{
			Verb:        "help",
			Description: "Show help (use --json for machine-readable)",
			Flags:       []Flag{{Name: "--json", Description: "Output as JSON"}},
			Stage:       "M0",
		},
		{
			Verb:        "version",
			Description: "Show version info",
			Stage:       "M0",
		},
		{
			Verb:        "read",
			Description: "Read file with encoding detection",
			Args:        []string{"<path>"},
			Flags: []Flag{
				{Name: "--encoding", Description: "Force encoding: auto|utf8|utf8bom|gbk|gb18030|utf16le|utf16be"},
				{Name: "--max-bytes", Description: "Max bytes to read"},
				{Name: "--offset", Description: "Start from line N (0-indexed)"},
				{Name: "--limit", Description: "Read N lines"},
			},
			Stage: "M1",
		},
		{
			Verb:        "write",
			Description: "Atomic file write",
			Args:        []string{"<path>", "<content>"},
			Flags: []Flag{
				{Name: "--encoding", Description: "Output encoding"},
				{Name: "--create-parents", Description: "Create parent directories"},
				{Name: "--expect-sha256", Description: "Optimistic lock: expected hash"},
				{Name: "--backup", Description: "Backup before write"},
				{Name: bypassFlag, Description: "Skip the terminal confirmation for a write into filex's own config dir; audited as a bypass"},
			},
			Stage: "M2",
		},
		{
			Verb:        "edit",
			Description: "Byte-level find/replace preserving encoding",
			Args:        []string{"<path>"},
			Flags: []Flag{
				{Name: "--find", Description: "Text to find"},
				{Name: "--replace", Description: "Replacement text"},
				{Name: "--all", Description: "Replace all occurrences"},
				{Name: "--dry-run", Description: "Show matches without modifying"},
				{Name: "--expect-sha256", Description: "Optimistic lock: expected hash"},
				{Name: "--backup", Description: "Backup before edit"},
				{Name: bypassFlag, Description: "Skip the terminal confirmation for an edit inside filex's own config dir; audited as a bypass"},
			},
			Stage: "M1",
		},
		{
			Verb:        "grep",
			Description: "Search with encoding awareness",
			Args:        []string{"<pattern>"},
			Flags: []Flag{
				{Name: "--paths", Description: "Paths to search"},
				{Name: "--encoding", Description: "Force encoding"},
				{Name: "--max", Description: "Max matches"},
				{Name: "--stream", Description: "Stream mode for large files"},
			},
			Stage: "M1/M5",
		},
		{
			Verb:        "stat",
			Description: "File metadata (size, mtime, encoding, lock status)",
			Args:        []string{"<path>"},
			Stage:       "M2",
		},
		{
			Verb:        "lock",
			Description: "Lock diagnosis (exclusive + deletable)",
			Args:        []string{"<path>"},
			Stage:       "M2",
		},
		{
			Verb:        "ls",
			Description: "List directory",
			Args:        []string{"<path>"},
			Flags: []Flag{
				{Name: "--depth", Description: "Recursion depth"},
				{Name: "--json", Description: "Output as JSON"},
			},
			Stage: "M2",
		},
		{
			Verb:        "hash",
			Description: "Compute checksum",
			Args:        []string{"<path>"},
			Flags: []Flag{
				{Name: "--algo", Description: "Hash algorithm (default: sha256)"},
			},
			Stage: "M2",
		},
		{
			Verb:        "rm",
			Description: "Remove (moved to ~/.filex/trash by default)",
			Args:        []string{"<path>"},
			Flags: []Flag{
				{Name: "--dry-run", Description: "Report what would happen; changes nothing"},
				{Name: "--purge", Description: "Permanently delete, unrecoverable (asks for confirmation on the terminal)"},
				{Name: bypassFlag, Description: "Skip the terminal confirmation; audited as a bypass and refused when guard.allow_bypass is false"},
			},
			Stage: "M2",
		},
		{
			Verb:        "tail",
			Description: "Read last N lines (encoding-aware)",
			Args:        []string{"<path>"},
			Flags: []Flag{
				{Name: "--lines", Description: "Number of lines (default: 10)"},
				{Name: "--encoding", Description: "Force encoding"},
				{Name: "--follow", Description: "Follow file (experimental)"},
			},
			Stage: "M5",
		},
		{
			Verb:        "env",
			Description: "Environment variables",
			Args:        []string{"<action>"},
			Flags: []Flag{
				{Name: "get", Description: "Get variable value"},
				{Name: "set", Description: "Set variable"},
				{Name: "list", Description: "List variables"},
				{Name: "unset", Description: "Delete variable"},
				{Name: "--scope", Description: "Scope: process|user|machine"},
				{Name: bypassFlag, Description: "Skip the terminal confirmation for a machine-scope write; audited as a bypass"},
			},
			Stage: "M3",
		},
		{
			Verb:        "backup",
			Description: "Backup management",
			Args:        []string{"<action>"},
			Flags: []Flag{
				{Name: "list", Description: "List backups"},
				{Name: "restore", Description: "Restore from backup"},
				{Name: "prune", Description: "Clean old backups"},
			},
			Stage: "M2",
		},
		{
			Verb:        "journal",
			Description: "Audit log analysis (read-only)",
			Args:        []string{"<stats|path>"},
			Flags: []Flag{
				{Name: "stats", Description: "Aggregate the audit log"},
				{Name: "path", Description: "Print the audit log path without creating it"},
				{Name: "--since", Description: "Start of window: duration counted back from now (48h) or a date"},
				{Name: "--until", Description: "End of window: duration counted back from now or a date"},
				{Name: "--session", Description: "Only records carrying this session id"},
				{Name: "--group-by", Description: "One section: session|verb|class|source (default: all)"},
				{Name: "--top", Description: "Rows per group (default: 20)"},
				{Name: "--json", Description: "Output as JSON"},
			},
			Stage: "M11",
		},
		{
			Verb:        "mcp",
			Description: "MCP server",
			Args:        []string{"<serve|install>"},
			Flags: []Flag{
				{Name: "serve", Description: "Start MCP stdio server"},
				{Name: "install", Description: "Install MCP config for an AI client"},
				{Name: "--client", Description: "Client: claude|claude-code|codex|cursor|vscode|windsurf|zcode"},
				{Name: "--profile", Description: "Tool profile: ro|rw (default ro)"},
			},
			Stage: "M6",
		},
		{
			Verb:        "upgrade",
			Description: "Upgrade to latest version",
			Flags: []Flag{
				{Name: "--check", Description: "Only check, don't upgrade"},
				{Name: "--yes", Description: "Non-interactive upgrade; treated as the audited bypass and refused when guard.allow_bypass is false"},
			},
			Stage: "M8",
		},
	}
}

func runHelp() *errs.Error {
	jsonMode := false
	for _, arg := range os.Args[2:] {
		if arg == "--json" {
			jsonMode = true
		}
	}

	if jsonMode {
		if err := errs.WriteSuccess(os.Stdout, getCommands()); err != nil {
			return errs.Newf(errs.EIO, "failed to write help: %v", err)
		}
		return nil
	}

	for _, cmd := range getCommands() {
		println(cmd.Verb, "-", cmd.Description, "["+cmd.Stage+"]")
	}
	return nil
}
