package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filex/internal/mcpserver"
)

func TestWriteMCPConfigCreatesAndMerges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "mcp.json")
	entry := map[string]any{"command": "filex.exe", "args": []string{"mcp", "serve"}}

	if err := writeMCPConfig(path, "filex", entry); err != nil {
		t.Fatalf("writeMCPConfig: %v", err)
	}

	var cfg map[string]any
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse written config: %v", err)
	}
	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers missing: %v", cfg)
	}
	if _, ok := servers["filex"]; !ok {
		t.Fatalf("filex server missing: %v", servers)
	}

	// A config with other servers and top-level keys must survive a merge.
	existing := `{"other": true, "mcpServers": {"sample": {"command": "x"}}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeMCPConfig(path, "filex", entry); err != nil {
		t.Fatalf("merge write: %v", err)
	}
	data, _ = os.ReadFile(path)
	cfg = map[string]any{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse merged config: %v", err)
	}
	if cfg["other"] != true {
		t.Error("unrelated top-level key lost")
	}
	servers, _ = cfg["mcpServers"].(map[string]any)
	if _, ok := servers["sample"]; !ok {
		t.Error("pre-existing server lost")
	}
	if _, ok := servers["filex"]; !ok {
		t.Error("filex server missing after merge")
	}
}

func TestWriteMCPConfigRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeMCPConfig(path, "filex", map[string]any{}); err == nil {
		t.Fatal("expected error for corrupt existing config")
	}
}

func TestWriteZCodeServerEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zcode", "cli", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"hooks": {"enabled": true}, "mcp": {"servers": {"envward": {"command": "e"}}}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := map[string]any{"command": "filex.exe", "args": []string{"mcp", "serve"}}
	if err := writeZCodeServerEntry(path, "filex", entry); err != nil {
		t.Fatalf("writeZCodeServerEntry: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	servers, ok := cfg["mcp"].(map[string]any)["servers"].(map[string]any)
	if !ok {
		t.Fatalf("mcp.servers missing: %v", cfg)
	}
	filex, ok := servers["filex"].(map[string]any)
	if !ok {
		t.Fatalf("filex server missing: %v", servers)
	}
	if filex["type"] != "stdio" {
		t.Errorf("type = %v, want stdio", filex["type"])
	}
	if filex["command"] != "filex.exe" {
		t.Errorf("command lost: %v", filex)
	}
	if _, ok := servers["envward"]; !ok {
		t.Error("pre-existing zcode server lost")
	}
	hooks, ok := cfg["hooks"].(map[string]any)
	if !ok || hooks["enabled"] != true {
		t.Error("unrelated section lost")
	}
}

func TestAppendCodexServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("model = \"gpt-5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := map[string]any{"command": `C:\Tools\filex.exe`, "args": []string{"mcp", "serve"}}
	if err := appendCodexServer(path, entry); err != nil {
		t.Fatalf("appendCodexServer: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "model = \"gpt-5\"") {
		t.Errorf("existing config lost: %s", content)
	}
	if !strings.Contains(content, "[mcp_servers.filex]") {
		t.Errorf("section missing: %s", content)
	}
	if !strings.Contains(content, `command = "C:\\Tools\\filex.exe"`) {
		t.Errorf("windows path not TOML-escaped: %s", content)
	}
	if !strings.Contains(content, `args = ["mcp", "serve"]`) {
		t.Errorf("args missing: %s", content)
	}

	before := content
	if err := appendCodexServer(path, entry); err == nil {
		t.Error("expected conflict when section already exists")
	}
	after, _ := os.ReadFile(path)
	if string(after) != before {
		t.Error("config modified despite conflict")
	}
}

func TestMCPInstallAllClients(t *testing.T) {
	home := t.TempDir()
	appData := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", appData)
	t.Setenv("XDG_CONFIG_HOME", appData)

	for _, client := range mcpClients {
		if err := runMCPInstall([]string{"--client=" + client, "--profile=rw"}); err != nil {
			t.Errorf("install %s: %v", client, err)
		}
	}

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{
		"claude":      filepath.Join(userConfigDir, "Claude", "claude_desktop_config.json"),
		"claude-code": filepath.Join(home, ".claude.json"),
		"codex":       filepath.Join(home, ".codex", "config.toml"),
		"cursor":      filepath.Join(home, ".cursor", "mcp.json"),
		"vscode":      filepath.Join(home, ".vscode", "mcp.json"),
		"windsurf":    filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"),
		"zcode":       filepath.Join(home, ".zcode", "cli", "config.json"),
	}
	for client, path := range expected {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s config missing: %v", client, err)
		}
	}

	data, err := os.ReadFile(expected["cursor"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "--profile=rw") {
		t.Errorf("profile flag missing from cursor config: %s", data)
	}
}

func TestMCPInstallUsageErrors(t *testing.T) {
	if err := runMCPInstall(nil); err == nil {
		t.Error("expected error when --client missing")
	}
	if err := runMCPInstall([]string{"--client=nope"}); err == nil {
		t.Error("expected error for unknown client")
	}
	if err := runMCPInstall([]string{"--client=cursor", "--profile=bogus"}); err == nil {
		t.Error("expected error for invalid profile")
	}
}

func TestParseProfileValue(t *testing.T) {
	profile, err := parseProfileValue("rw")
	if err != nil || profile != mcpserver.ProfileRW {
		t.Errorf("rw = %v, %v; want rw", profile, err)
	}
	if _, err := parseProfileValue("bogus"); err == nil {
		t.Error("expected error for invalid profile")
	}
}
