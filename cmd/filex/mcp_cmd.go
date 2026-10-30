package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"filex/internal/errs"
	"filex/internal/guard"
	"filex/internal/mcpserver"
	"filex/internal/ops"
)

// mcpClients lists every AI client `filex mcp install` can configure.
var mcpClients = []string{"claude", "claude-code", "codex", "cursor", "vscode", "windsurf", "zcode"}

func runMCP(args []string) *errs.Error {
	if len(args) == 0 {
		return errs.WithHint(
			errs.New(errs.EUsage, "mcp requires a subcommand"),
			"usage: filex mcp <serve|install> [args...]",
		)
	}

	switch args[0] {
	case "serve":
		return runMCPServe(args[1:])
	case "install":
		return runMCPInstall(args[1:])
	default:
		return errs.Newf(errs.EUsage, "unknown mcp subcommand: %s", args[0])
	}
}

// parseProfileValue validates a --profile value shared by serve and install.
func parseProfileValue(value string) (mcpserver.Profile, *errs.Error) {
	profile, ok := mcpserver.ParseProfile(value)
	if !ok {
		return "", errs.Newf(errs.EUsage, "invalid profile: %s (valid: ro, rw)", value)
	}
	return profile, nil
}

func runMCPServe(args []string) *errs.Error {
	profile := mcpserver.ProfileRO
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--profile" && i+1 < len(args):
			i++
			p, err := parseProfileValue(args[i])
			if err != nil {
				return err
			}
			profile = p
		case strings.HasPrefix(args[i], "--profile="):
			p, err := parseProfileValue(strings.TrimPrefix(args[i], "--profile="))
			if err != nil {
				return err
			}
			profile = p
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A model at the other end of a pipe is not a human at a terminal: bind the
	// deny-only runtime before the server can touch a file. Each tools/call
	// re-binds it with the client's name.
	ops.BindRuntime(ops.Runtime{
		Actor:       "mcp",
		Interactive: false,
		Confirmer:   guard.DenyConfirmer{},
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	server := mcpserver.NewServer(profile)
	if err := server.Serve(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errs.Newf(errs.EInternal, "mcp server error: %v", err)
	}

	return nil
}

func runMCPInstall(args []string) *errs.Error {
	var client string
	profile := mcpserver.ProfileRO
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--client" && i+1 < len(args):
			i++
			client = args[i]
		case strings.HasPrefix(args[i], "--client="):
			client = strings.TrimPrefix(args[i], "--client=")
		case args[i] == "--profile" && i+1 < len(args):
			i++
			p, err := parseProfileValue(args[i])
			if err != nil {
				return err
			}
			profile = p
		case strings.HasPrefix(args[i], "--profile="):
			p, err := parseProfileValue(strings.TrimPrefix(args[i], "--profile="))
			if err != nil {
				return err
			}
			profile = p
		}
	}

	if client == "" {
		return errs.WithHint(
			errs.New(errs.EUsage, "--client is required"),
			fmt.Sprintf("usage: filex mcp install --client=<%s> [--profile=<ro|rw>]", strings.Join(mcpClients, "|")),
		)
	}

	exe, err := os.Executable()
	if err != nil {
		return errs.Newf(errs.EIO, "cannot determine executable path: %v", err)
	}

	serveArgs := []string{"mcp", "serve"}
	if profile != mcpserver.ProfileRO {
		serveArgs = append(serveArgs, "--profile="+profile.String())
	}
	serverConfig := map[string]any{
		"command": exe,
		"args":    serveArgs,
	}

	switch client {
	case "claude":
		return installClaudeDesktop(serverConfig)
	case "claude-code":
		return installClaudeCode(serverConfig)
	case "codex":
		return installCodex(serverConfig)
	case "cursor":
		return installCursor(serverConfig)
	case "vscode":
		return installVSCode(serverConfig)
	case "windsurf":
		return installWindsurf(serverConfig)
	case "zcode":
		return installZCode(serverConfig)
	default:
		return errs.Newf(errs.EUsage, "unsupported client: %s (valid: %s)", client, strings.Join(mcpClients, ", "))
	}
}

// homeConfigPath resolves a path under the user home directory.
func homeConfigPath(elem ...string) (string, *errs.Error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errs.Newf(errs.EIO, "cannot get home dir: %v", err)
	}
	return filepath.Join(append([]string{home}, elem...)...), nil
}

// installClaudeDesktop writes the server into Claude Desktop's config.
// os.UserConfigDir resolves to the directory Claude Desktop reads on each
// platform: %AppData% on Windows, ~/Library/Application Support on macOS,
// $XDG_CONFIG_HOME (~/.config) on Linux.
func installClaudeDesktop(config map[string]any) *errs.Error {
	base, err := os.UserConfigDir()
	if err != nil {
		return errs.Newf(errs.EIO, "cannot determine user config dir: %v", err)
	}
	return writeMCPConfig(filepath.Join(base, "Claude", "claude_desktop_config.json"), "filex", config)
}

// installClaudeCode writes a user-scope MCP entry into the Claude Code CLI's
// config (~/.claude.json), preserving the other state the CLI keeps there.
func installClaudeCode(config map[string]any) *errs.Error {
	path, err := homeConfigPath(".claude.json")
	if err != nil {
		return err
	}
	return writeMCPConfig(path, "filex", config)
}

func installCodex(config map[string]any) *errs.Error {
	path, err := homeConfigPath(".codex", "config.toml")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errs.Newf(errs.EIO, "cannot create config dir: %v", err)
	}
	return appendCodexServer(path, config)
}

func installCursor(config map[string]any) *errs.Error {
	path, err := homeConfigPath(".cursor", "mcp.json")
	if err != nil {
		return err
	}
	return writeMCPConfig(path, "filex", config)
}

// installVSCode targets VS Code's user-level MCP config (~/.vscode/mcp.json);
// workspace-level .vscode/mcp.json stays under the user's control.
func installVSCode(config map[string]any) *errs.Error {
	path, err := homeConfigPath(".vscode", "mcp.json")
	if err != nil {
		return err
	}
	return writeMCPConfig(path, "filex", config)
}

func installWindsurf(config map[string]any) *errs.Error {
	path, err := homeConfigPath(".codeium", "windsurf", "mcp_config.json")
	if err != nil {
		return err
	}
	return writeMCPConfig(path, "filex", config)
}

// installZCode registers the server in ZCode's user config
// (~/.zcode/cli/config.json), which nests MCP servers under mcp.servers.
func installZCode(config map[string]any) *errs.Error {
	path, err := homeConfigPath(".zcode", "cli", "config.json")
	if err != nil {
		return err
	}
	return writeZCodeServerEntry(path, "filex", config)
}

// writeMCPConfig merges a server entry into the common "mcpServers" config
// layout used by Claude Desktop, Claude Code, Cursor, VS Code and Windsurf.
func writeMCPConfig(path, name string, serverConfig map[string]any) *errs.Error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errs.Newf(errs.EIO, "cannot create config dir: %v", err)
	}
	existing, err := loadJSONConfig(path)
	if err != nil {
		return errs.Newf(errs.EIO, "read config: %v", err)
	}

	mcpServers, _ := existing["mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = make(map[string]any)
	}
	mcpServers[name] = serverConfig
	existing["mcpServers"] = mcpServers

	if err := saveJSONConfig(path, existing); err != nil {
		return errs.Newf(errs.EIO, "write config: %v", err)
	}
	fmt.Fprintf(os.Stderr, "MCP config written to %s\n", path)
	return nil
}

// writeZCodeServerEntry merges a server entry into ZCode's nested
// "mcp"."servers" config structure instead of the common "mcpServers" key.
func writeZCodeServerEntry(path, name string, serverConfig map[string]any) *errs.Error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errs.Newf(errs.EIO, "cannot create config dir: %v", err)
	}
	existing, err := loadJSONConfig(path)
	if err != nil {
		return errs.Newf(errs.EIO, "read config: %v", err)
	}

	mcpSection, _ := existing["mcp"].(map[string]any)
	if mcpSection == nil {
		mcpSection = make(map[string]any)
	}
	servers, _ := mcpSection["servers"].(map[string]any)
	if servers == nil {
		servers = make(map[string]any)
	}

	entry := make(map[string]any, len(serverConfig)+1)
	entry["type"] = "stdio"
	for key, value := range serverConfig {
		entry[key] = value
	}
	servers[name] = entry
	mcpSection["servers"] = servers
	existing["mcp"] = mcpSection

	if err := saveJSONConfig(path, existing); err != nil {
		return errs.Newf(errs.EIO, "write config: %v", err)
	}
	fmt.Fprintf(os.Stderr, "MCP config written to %s\n", path)
	return nil
}

// appendCodexServer appends a [mcp_servers.filex] section to Codex's TOML
// config. Codex config is TOML, so the section is appended rather than merged
// to never rewrite the user's file; an existing filex section is refused.
func appendCodexServer(path string, config map[string]any) *errs.Error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return errs.Newf(errs.EIO, "read config: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "[mcp_servers.filex]") {
		return errs.WithHint(
			errs.Newf(errs.EConflict, "%s already defines mcp_servers.filex", path),
			"edit the existing section manually instead",
		)
	}

	var section strings.Builder
	section.WriteString("[mcp_servers.filex]\n")
	fmt.Fprintf(&section, "command = %q\n", config["command"])
	section.WriteString("args = [")
	if args, ok := config["args"].([]string); ok {
		for i, arg := range args {
			if i > 0 {
				section.WriteString(", ")
			}
			fmt.Fprintf(&section, "%q", arg)
		}
	}
	section.WriteString("]\n")

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return errs.Newf(errs.EIO, "open config: %v", err)
	}
	defer file.Close()
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		if _, err := file.WriteString("\n"); err != nil {
			return errs.Newf(errs.EIO, "write config: %v", err)
		}
	}
	if _, err := file.WriteString(section.String()); err != nil {
		return errs.Newf(errs.EIO, "write config: %v", err)
	}

	fmt.Fprintf(os.Stderr, "MCP config written to %s\n", path)
	return nil
}

// loadJSONConfig reads a JSON object config file, returning an empty map when
// the file does not exist. A file that exists but fails to parse is an error,
// so a corrupt config is never silently overwritten.
func loadJSONConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}
		return nil, err
	}
	config := make(map[string]any)
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return config, nil
}

// saveJSONConfig writes a config object back as indented JSON.
func saveJSONConfig(path string, config map[string]any) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
