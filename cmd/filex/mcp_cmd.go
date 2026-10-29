package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"filex/internal/errs"
	"filex/internal/mcpserver"
)

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

func runMCPServe(args []string) *errs.Error {
	profile := mcpserver.ProfileRO
	for i := 0; i < len(args); i++ {
		if args[i] == "--profile" && i+1 < len(args) {
			i++
			p, ok := mcpserver.ParseProfile(args[i])
			if !ok {
				return errs.Newf(errs.EUsage, "invalid profile: %s (valid: ro, rw)", args[i])
			}
			profile = p
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	for i := 0; i < len(args); i++ {
		if args[i] == "--client" && i+1 < len(args) {
			i++
			client = args[i]
		}
	}

	if client == "" {
		return errs.WithHint(
			errs.New(errs.EUsage, "--client is required"),
			"usage: filex mcp install --client=<claude|cursor|vscode>",
		)
	}

	exe, err := os.Executable()
	if err != nil {
		return errs.Newf(errs.EIO, "cannot determine executable path: %v", err)
	}

	config := map[string]any{
		"command": exe,
		"args":    []string{"mcp", "serve"},
	}

	switch client {
	case "claude":
		return installClaudeDesktop(config)
	case "cursor":
		return installCursor(config)
	case "vscode":
		return installVSCode(config)
	default:
		return errs.Newf(errs.EUsage, "unsupported client: %s (valid: claude, cursor, vscode)", client)
	}
}

func installClaudeDesktop(config map[string]any) *errs.Error {
	home, err := os.UserHomeDir()
	if err != nil {
		return errs.Newf(errs.EIO, "cannot get home dir: %v", err)
	}

	configDir := filepath.Join(home, "Library", "Application Support", "Claude")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		configDir = filepath.Join(home, ".config", "Claude")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return errs.Newf(errs.EIO, "cannot create config dir: %v", err)
		}
	}

	configPath := filepath.Join(configDir, "claude_desktop_config.json")
	return writeMCPConfig(configPath, "filex", config)
}

func installCursor(config map[string]any) *errs.Error {
	home, err := os.UserHomeDir()
	if err != nil {
		return errs.Newf(errs.EIO, "cannot get home dir: %v", err)
	}

	configDir := filepath.Join(home, ".cursor")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errs.Newf(errs.EIO, "cannot create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "mcp.json")
	return writeMCPConfig(configPath, "filex", config)
}

func installVSCode(config map[string]any) *errs.Error {
	home, err := os.UserHomeDir()
	if err != nil {
		return errs.Newf(errs.EIO, "cannot get home dir: %v", err)
	}

	configDir := filepath.Join(home, ".vscode")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errs.Newf(errs.EIO, "cannot create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "mcp.json")
	return writeMCPConfig(configPath, "filex", config)
}

func writeMCPConfig(path, name string, serverConfig map[string]any) *errs.Error {
	existing := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &existing)
	}

	mcpServers, _ := existing["mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = make(map[string]any)
	}
	mcpServers[name] = serverConfig
	existing["mcpServers"] = mcpServers

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return errs.Newf(errs.EIO, "marshal config: %v", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return errs.Newf(errs.EIO, "write config: %v", err)
	}

	fmt.Fprintf(os.Stderr, "MCP config written to %s\n", path)
	return nil
}
