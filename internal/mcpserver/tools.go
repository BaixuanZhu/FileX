package mcpserver

import (
	"context"
	"fmt"
	"strconv"

	"filex/internal/ops"
	"filex/internal/winenv"
)

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func getToolDefinitions(profile Profile) []ToolDefinition {
	allTools := []ToolDefinition{
		{
			Name:        "read",
			Description: "Read file with encoding detection",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string", "description": "File path"},
					"encoding":  map[string]any{"type": "string", "description": "Force encoding: auto|utf8|utf8bom|gbk|gb18030|utf16le|utf16be"},
					"max_bytes": map[string]any{"type": "integer", "description": "Max bytes to read"},
					"offset":    map[string]any{"type": "integer", "description": "Start from line N (0-indexed)"},
					"limit":     map[string]any{"type": "integer", "description": "Read N lines"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "write",
			Description: "Atomic file write with backup support",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":           map[string]any{"type": "string", "description": "File path"},
					"content":        map[string]any{"type": "string", "description": "Content to write"},
					"encoding":       map[string]any{"type": "string", "description": "Output encoding"},
					"create_parents": map[string]any{"type": "boolean", "description": "Create parent directories"},
					"expect_sha256":  map[string]any{"type": "string", "description": "Optimistic lock: expected hash"},
					"backup":         map[string]any{"type": "boolean", "description": "Backup before write"},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "edit",
			Description: "Byte-level find/replace preserving encoding",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":          map[string]any{"type": "string", "description": "File path"},
					"find":          map[string]any{"type": "string", "description": "Text to find"},
					"replace":       map[string]any{"type": "string", "description": "Replacement text"},
					"all":           map[string]any{"type": "boolean", "description": "Replace all occurrences"},
					"dry_run":       map[string]any{"type": "boolean", "description": "Show matches without modifying"},
					"expect_sha256": map[string]any{"type": "string", "description": "Optimistic lock: expected hash"},
					"encoding":      map[string]any{"type": "string", "description": "Force encoding"},
					"backup":        map[string]any{"type": "boolean", "description": "Backup before edit"},
				},
				"required": []string{"path", "find", "replace"},
			},
		},
		{
			Name:        "grep",
			Description: "Search with encoding awareness",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "Search pattern (regex)"},
					"paths":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Paths to search"},
					"max":     map[string]any{"type": "integer", "description": "Max matches"},
					"stream":  map[string]any{"type": "boolean", "description": "Stream mode for large files"},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "stat",
			Description: "File metadata (size, mtime, encoding, lock status)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "File path"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "lock",
			Description: "Lock diagnosis (exclusive + deletable)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "File path"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "ls",
			Description: "List directory",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string", "description": "Directory path"},
					"depth": map[string]any{"type": "integer", "description": "Recursion depth"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "rm",
			Description: "Remove file (moved to ~/.filex/trash by default)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "File path"},
					"purge":   map[string]any{"type": "boolean", "description": "Permanently delete, unrecoverable"},
					"dry_run": map[string]any{"type": "boolean", "description": "Report what would happen; changes nothing"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "tail",
			Description: "Read last N lines (encoding-aware)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "File path"},
					"lines":  map[string]any{"type": "integer", "description": "Number of lines (default: 10)"},
					"follow": map[string]any{"type": "boolean", "description": "Follow file for new content"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "backup",
			Description: "Backup management (list/restore/prune)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "enum": []string{"list", "restore", "prune"}, "description": "Action to perform"},
					"id":     map[string]any{"type": "string", "description": "Backup ID (for restore)"},
					"path":   map[string]any{"type": "string", "description": "Filter by path (for list)"},
				},
				"required": []string{"action"},
			},
		},
		{
			Name:        "env",
			Description: "Environment variables (get/set/list/unset)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "enum": []string{"get", "set", "list", "unset"}, "description": "Action to perform"},
					"name":   map[string]any{"type": "string", "description": "Variable name"},
					"value":  map[string]any{"type": "string", "description": "Variable value (for set)"},
					"scope":  map[string]any{"type": "string", "enum": []string{"process", "user", "machine"}, "description": "Scope"},
				},
				"required": []string{"action"},
			},
		},
	}

	var filtered []ToolDefinition
	for _, tool := range allTools {
		if profile.AllowsTool(tool.Name) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func dispatchTool(ctx context.Context, profile Profile, name string, args map[string]any) (any, error) {
	switch name {
	case "read":
		return dispatchRead(args)
	case "write":
		return dispatchWrite(args)
	case "edit":
		return dispatchEdit(args)
	case "grep":
		return dispatchGrep(args)
	case "stat":
		return dispatchStat(args)
	case "lock":
		return dispatchLock(args)
	case "ls":
		return dispatchLs(args)
	case "rm":
		return dispatchRm(args)
	case "tail":
		return dispatchTail(args)
	case "backup":
		return dispatchBackup(profile, args)
	case "env":
		return dispatchEnv(profile, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func getString(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(args map[string]any, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case string:
			if i, err := strconv.Atoi(n); err == nil {
				return i
			}
		}
	}
	return defaultVal
}

func getBool(args map[string]any, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getStringSlice(args map[string]any, key string) []string {
	if v, ok := args[key]; ok {
		if arr, ok := v.([]any); ok {
			var result []string
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}

func dispatchRead(args map[string]any) (any, error) {
	path := getString(args, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	result, err := ops.Read(ops.ReadParams{
		Path:     path,
		Encoding: getString(args, "encoding"),
		MaxBytes: int64(getInt(args, "max_bytes", 0)),
		Offset:   getInt(args, "offset", 0),
		Limit:    getInt(args, "limit", 0),
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func dispatchWrite(args map[string]any) (any, error) {
	path := getString(args, "path")
	content := getString(args, "content")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	result, err := ops.Write(ops.WriteParams{
		Path:          path,
		Content:       content,
		Encoding:      getString(args, "encoding"),
		CreateParents: getBool(args, "create_parents"),
		ExpectSHA256:  getString(args, "expect_sha256"),
		Backup:        getBool(args, "backup"),
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func dispatchEdit(args map[string]any) (any, error) {
	path := getString(args, "path")
	find := getString(args, "find")
	if path == "" || find == "" {
		return nil, fmt.Errorf("path and find are required")
	}
	result, err := ops.Edit(ops.EditParams{
		Path:         path,
		Find:         find,
		Replace:      getString(args, "replace"),
		All:          getBool(args, "all"),
		DryRun:       getBool(args, "dry_run"),
		ExpectSHA256: getString(args, "expect_sha256"),
		Encoding:     getString(args, "encoding"),
		Backup:       getBool(args, "backup"),
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func dispatchGrep(args map[string]any) (any, error) {
	pattern := getString(args, "pattern")
	if pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}
	paths := getStringSlice(args, "paths")
	if len(paths) == 0 {
		paths = []string{"."}
	}
	result, err := ops.Grep(ops.GrepParams{
		Pattern: pattern,
		Paths:   paths,
		Max:     getInt(args, "max", 100),
		Stream:  getBool(args, "stream"),
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func dispatchStat(args map[string]any) (any, error) {
	path := getString(args, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	result, err := ops.Stat(path)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func dispatchLock(args map[string]any) (any, error) {
	path := getString(args, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	result, err := ops.Lock(path)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func dispatchLs(args map[string]any) (any, error) {
	path := getString(args, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	result, err := ops.Ls(path, getInt(args, "depth", 1))
	if err != nil {
		return nil, err
	}
	return result, nil
}

func dispatchRm(args map[string]any) (any, error) {
	path := getString(args, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	// A legacy "yes" from the client is ignored: a model must not authorise its own destructive request.
	result, err := ops.Rm(ops.RmParams{
		Path:   path,
		Purge:  getBool(args, "purge"),
		DryRun: getBool(args, "dry_run"),
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func dispatchTail(args map[string]any) (any, error) {
	path := getString(args, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	result, err := ops.Tail(ops.TailParams{
		Path:   path,
		Lines:  getInt(args, "lines", 10),
		Follow: getBool(args, "follow"),
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func dispatchBackup(profile Profile, args map[string]any) (any, error) {
	action := getString(args, "action")
	if !profile.AllowsBackupAction(action) {
		return nil, fmt.Errorf("backup action %q not allowed in %s profile", action, profile)
	}
	switch action {
	case "list":
		filter := getString(args, "path")
		result, err := ops.BackupList(filter)
		if err != nil {
			return nil, err
		}
		return result, nil
	case "restore":
		id := getString(args, "id")
		if id == "" {
			return nil, fmt.Errorf("id is required for restore")
		}
		result, err := ops.BackupRestore(id)
		if err != nil {
			return nil, err
		}
		return result, nil
	case "prune":
		result, err := ops.BackupPrune(getInt(args, "retention", 3))
		if err != nil {
			return nil, err
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unknown backup action: %s", action)
	}
}

func dispatchEnv(profile Profile, args map[string]any) (any, error) {
	action := getString(args, "action")
	if !profile.AllowsEnvAction(action) {
		return nil, fmt.Errorf("env action %q not allowed in %s profile", action, profile)
	}
	scopeStr := getString(args, "scope")
	if scopeStr == "" {
		scopeStr = "user"
	}
	scope, err := winenv.ParseScope(scopeStr)
	if err != nil {
		return nil, err
	}

	switch action {
	case "get":
		name := getString(args, "name")
		if name == "" {
			return nil, fmt.Errorf("name is required for get")
		}
		result, err := ops.EnvGet(scope, name)
		if err != nil {
			return nil, err
		}
		return result, nil
	case "set":
		name := getString(args, "name")
		value := getString(args, "value")
		if name == "" {
			return nil, fmt.Errorf("name is required for set")
		}
		result, err := ops.EnvSet(ops.EnvSetParams{
			Scope: scope,
			Name:  name,
			Value: value,
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	case "list":
		result, err := ops.EnvList(scope)
		if err != nil {
			return nil, err
		}
		return result, nil
	case "unset":
		name := getString(args, "name")
		if name == "" {
			return nil, fmt.Errorf("name is required for unset")
		}
		result, err := ops.EnvUnset(scope, name)
		if err != nil {
			return nil, err
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unknown env action: %s", action)
	}
}
