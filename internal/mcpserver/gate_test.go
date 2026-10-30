package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filex/internal/journal"
)

// isolate points HOME, USERPROFILE and the working directory at a temp dir, so
// policy checks, trash and journal all stay inside the test.
func isolate(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}
	t.Setenv("HOME", resolved)
	t.Setenv("USERPROFILE", resolved)
	t.Chdir(resolved)

	return resolved
}

// newTestServer stands up a server that has completed initialize as the named
// client, which is when the runtime gets bound and the actor becomes known.
func newTestServer(t *testing.T, clientName string) *Server {
	t.Helper()

	s := NewServer(ProfileRW)
	params := mustParams(t, clientName)
	if resp := s.handleInitialize(&jsonrpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: params}); resp == nil || resp.Error != nil {
		t.Fatalf("initialize failed: %+v", resp)
	}
	return s
}

func callTool(t *testing.T, s *Server, name string, args map[string]any) map[string]any {
	t.Helper()

	params, err := json.Marshal(map[string]any{"name": name, "arguments": args})
	if err != nil {
		t.Fatalf("encode tool params: %v", err)
	}
	resp := s.handleToolsCall(context.Background(), &jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params:  params,
	})
	if resp == nil {
		t.Fatal("tools/call produced no response")
	}
	if resp.Error != nil {
		t.Fatalf("transport-level error: %s", resp.Error.Message)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is %#v, want an object", resp.Result)
	}
	return result
}

func toolText(t *testing.T, result map[string]any) string {
	t.Helper()

	contents, ok := result["content"].([]map[string]any)
	if !ok || len(contents) == 0 {
		t.Fatalf("result has no content: %#v", result)
	}
	text, _ := contents[0]["text"].(string)
	return text
}

func readFileOrFail(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func journalLines(t *testing.T, root string) []map[string]any {
	t.Helper()

	data := readFileOrFail(t, filepath.Join(root, ".filex", "journal", "filex.jsonl"))
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		if line == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("decode journal line %q: %v", line, err)
		}
		out = append(out, e)
	}
	return out
}

func entriesWithOp(t *testing.T, root, op string) []map[string]any {
	t.Helper()

	var out []map[string]any
	for _, e := range journalLines(t, root) {
		if e["op"] == op {
			out = append(out, e)
		}
	}
	return out
}

// A model cannot approve its own irreversible request: there is no terminal to
// answer, so purge is refused and the file is untouched.
func TestPurgeDeniedOverMCP(t *testing.T) {
	root := isolate(t)
	path := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := newTestServer(t, "claude")
	result := callTool(t, s, "rm", map[string]any{"path": path, "purge": true})

	if result["isError"] != true {
		t.Fatalf("purge over MCP was not an error: %#v", result)
	}
	text := toolText(t, result)
	// The refusal has to be actionable without the model guessing.
	for _, want := range []string{"permanent delete is unrecoverable", "safer alternative"} {
		if !strings.Contains(text, want) {
			t.Fatalf("denial text %q does not carry %q", text, want)
		}
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("denied purge removed the file: %v", err)
	}
}

// The tool schema no longer offers "yes", but an older client may still send it.
// It must be accepted and then change nothing.
func TestLegacyYesGrantsNothingOverMCP(t *testing.T) {
	root := isolate(t)
	path := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := newTestServer(t, "claude")
	result := callTool(t, s, "rm", map[string]any{"path": path, "purge": true, "yes": true})

	if result["isError"] != true {
		t.Fatalf(`a legacy "yes" authorised a purge: %#v`, result)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(`purge with "yes" removed the file: %v`, err)
	}
}

// Dry-run stays available: it is how an agent learns what it would have done.
func TestDryRunOverMCP(t *testing.T) {
	root := isolate(t)
	path := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := newTestServer(t, "claude")
	result := callTool(t, s, "rm", map[string]any{"path": path, "purge": true, "dry_run": true})

	if result["isError"] == true {
		t.Fatalf("dry run was refused: %s", toolText(t, result))
	}
	text := toolText(t, result)
	if !strings.Contains(text, "permanent_delete") {
		t.Fatalf("dry run did not report the action: %s", text)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("dry run removed the file: %v", err)
	}
}

// A refusal is only useful later if it says who asked. The actor comes from the
// client's own initialize.
func TestDenialIsAuditedWithTheClientActor(t *testing.T) {
	root := isolate(t)
	path := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := newTestServer(t, "cursor")
	callTool(t, s, "rm", map[string]any{"path": path, "purge": true})

	denials := entriesWithOp(t, root, "rm")
	if len(denials) != 1 {
		t.Fatalf("want one rm record, got %d: %+v", len(denials), denials)
	}
	e := denials[0]
	if e["result"] != "denied" {
		t.Fatalf("result = %v, want denied", e["result"])
	}
	if e["actor"] != "mcp:cursor" {
		t.Fatalf("actor = %v, want mcp:cursor", e["actor"])
	}
	if e["guard_class"] != "purge_delete" {
		t.Fatalf("guard_class = %v, want purge_delete", e["guard_class"])
	}
	if e["decision_reason"] == "" || e["decision_reason"] == nil {
		t.Fatalf("denial record lost its reason: %+v", e)
	}

	// initialize itself is on the record, so a session can be traced to a client.
	inits := entriesWithOp(t, root, "mcp_initialize")
	if len(inits) != 1 {
		t.Fatalf("want one mcp_initialize record, got %d: %+v", len(inits), inits)
	}
	clientInfo, _ := inits[0]["client_info"].(string)
	if !strings.Contains(clientInfo, "cursor") {
		t.Fatalf("mcp_initialize did not name the client: %+v", inits[0])
	}
}

func TestIdentity(t *testing.T) {
	s := NewServer(ProfileRO)

	// Before initialize there is neither a client name nor a connection id, and
	// the honest answer is an unnamed actor with no session at all.
	actor, sessionID, sessionSource := s.identity()
	if actor != "mcp" || sessionID != "" || sessionSource != "" {
		t.Fatalf("before initialize: got %q/%q/%q, want mcp with no session", actor, sessionID, sessionSource)
	}

	s.clientInfo = map[string]any{"name": "claude-code"}
	s.sessionID = journal.NewSessionID()
	actor, sessionID, sessionSource = s.identity()
	if actor != "mcp:claude-code" {
		t.Fatalf("actor = %q, want mcp:claude-code", actor)
	}
	if sessionSource != journal.SessionSourceMCPConnection {
		t.Fatalf("session_source = %q, want %q", sessionSource, journal.SessionSourceMCPConnection)
	}
	if len(sessionID) != 32 {
		t.Fatalf("session_id = %q, want 32 hex characters", sessionID)
	}

	// An unnamed client must not borrow another client's identity.
	s.clientInfo = map[string]any{"version": "0.1"}
	if actor, _, _ := s.identity(); actor != "mcp" {
		t.Fatalf("unnamed client: actor = %q, want mcp", actor)
	}
}

// Some hosts re-send initialize during a single connection. Grouping only works
// if the id survives, so a repeated handshake must not start a new session.
func TestSessionIDSurvivesAReinitialize(t *testing.T) {
	isolate(t)
	s := newTestServer(t, "claude")
	replay := &jsonrpcRequest{JSONRPC: "2.0", ID: 4, Method: "initialize", Params: mustParams(t, "claude")}
	if resp := s.handleInitialize(replay); resp.Error != nil {
		t.Fatalf("repeated initialize failed: %+v", resp.Error)
	}
	_, first, _ := s.identity()
	if resp := s.handleInitialize(replay); resp.Error != nil {
		t.Fatalf("third initialize failed: %+v", resp.Error)
	}
	_, second, _ := s.identity()
	if first != second {
		t.Fatalf("session id changed across handshakes on one connection: %q then %q", first, second)
	}
}

func mustParams(t *testing.T, clientName string) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(initializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo:      map[string]any{"name": clientName, "version": "0.1"},
	})
	if err != nil {
		t.Fatalf("encode initialize params: %v", err)
	}
	return raw
}

// The session id has to reach the operations, not just the handshake — otherwise
// the aggregation can only group the initialize records.
func TestOperationsCarryTheConnectionSession(t *testing.T) {
	root := isolate(t)
	path := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := newTestServer(t, "claude")
	_, want, _ := s.identity()

	callTool(t, s, "rm", map[string]any{"path": path, "purge": true})
	callTool(t, s, "rm", map[string]any{"path": path})

	records := append(entriesWithOp(t, root, "rm"), entriesWithOp(t, root, "mcp_initialize")...)
	if len(records) < 2 {
		t.Fatalf("want audited operations, got %+v", records)
	}
	for _, e := range records {
		if e["session_id"] != want {
			t.Fatalf("%v: session_id = %v, want the connection id %v", e["op"], e["session_id"], want)
		}
		if e["session_source"] != journal.SessionSourceMCPConnection {
			t.Fatalf("%v: session_source = %v, want %q", e["op"], e["session_source"], journal.SessionSourceMCPConnection)
		}
	}
}
