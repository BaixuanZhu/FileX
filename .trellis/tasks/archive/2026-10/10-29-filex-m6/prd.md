# M6: MCP adapter — stdlib JSON-RPC 2.0, profiles, tool mapping

## Goal

Implement stdio MCP server mapping 11 tools to ops, with ro/rw profiles. `filex mcp serve` starts server; `filex mcp install` writes client config.

## Requirements

- JSON-RPC 2.0 over stdio (no third-party MCP SDK)
- 11 tools: read, write, edit, grep, stat, lock, ls, rm, tail, backup, env
- Profile ro (default): read grep stat lock ls tail env(get/list) backup(list)
- Profile rw: all tools
- `initialize` records clientInfo to journal
- Context cancellation for long operations
- `filex mcp serve` — start stdio server
- `filex mcp install --client=claude|cursor|vscode` — write config

## Acceptance Criteria

- [ ] MCP client can call all 11 tools
- [ ] ro profile restricts to read-only tools
- [ ] rw profile exposes all tools
- [ ] clientInfo from initialize recorded in journal
- [ ] go build/test/vet pass

## Notes

- Pure stdlib implementation, no MCP SDK dependency
- Content-Length header framing for stdio transport
- Tool input schemas as JSON Schema objects
