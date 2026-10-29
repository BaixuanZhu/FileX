package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	AllowedDirs      []string `json:"allowed_dirs,omitempty"`
	DenyPatterns     []string `json:"deny_patterns,omitempty"`
	BackupBeforeWrite bool    `json:"backup_before_write"`
}

func DefaultConfig() Config {
	return Config{
		DenyPatterns: []string{
			`Windows\**`,
			`Program Files\**`,
			`Program Files (x86)\**`,
			`.git\objects\**`,
		},
		BackupBeforeWrite: true,
	}
}

func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultConfig(), nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".filex", "config.json"))
	if err != nil {
		return DefaultConfig(), nil
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), nil
	}
	if len(cfg.DenyPatterns) == 0 {
		cfg.DenyPatterns = DefaultConfig().DenyPatterns
	}
	return cfg, nil
}

type Result struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

func Allowed() Result      { return Result{Allowed: true} }
func Denied(reason string) Result { return Result{Allowed: false, Reason: reason} }

func Check(path string, cfg Config) Result {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Denied("cannot resolve absolute path: " + err.Error())
	}

	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return Denied("cannot resolve symlinks: " + err.Error())
		}
		resolved = absPath
	}

	resolvedRoot := resolveRoot(resolved)
	absRoot := resolveRoot(absPath)

	if isDeniedByPatterns(resolved, cfg.DenyPatterns) ||
		isDeniedByPatterns(absPath, cfg.DenyPatterns) ||
		isDeniedByPatterns(resolvedRoot, cfg.DenyPatterns) ||
		isDeniedByPatterns(absRoot, cfg.DenyPatterns) {
		return Denied("path matches deny pattern")
	}

	for _, allowed := range cfg.AllowedDirs {
		absAllowed, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		resolvedAllowed, err := filepath.EvalSymlinks(absAllowed)
		if err != nil {
			resolvedAllowed = absAllowed
		}
		if isUnder(resolved, resolvedAllowed) || isUnder(absPath, absAllowed) {
			return Allowed()
		}
	}

	cwd, err := os.Getwd()
	if err == nil && (isUnder(resolved, cwd) || isUnder(absPath, cwd)) {
		return Allowed()
	}

	if len(cfg.AllowedDirs) > 0 {
		return Denied("path is not under any allowed directory")
	}

	return Denied("path is outside current working directory")
}

func resolveRoot(path string) string {
	vol := filepath.VolumeName(path)
	if vol == "" {
		return path
	}
	rest := path[len(vol):]
	rest = strings.TrimLeft(rest, `/\`)
	parts := strings.Split(rest, string(filepath.Separator))
	if len(parts) > 0 && parts[0] != "" {
		return vol + string(filepath.Separator) + parts[0]
	}
	return vol + string(filepath.Separator)
}

func isUnder(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

func isDeniedByPatterns(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchPattern(path, pattern) {
			return true
		}
	}
	return false
}

func matchPattern(path, pattern string) bool {
	pathParts := splitPath(path)
	patternParts := splitPath(pattern)
	for i := 0; i <= len(pathParts); i++ {
		if matchParts(pathParts[i:], patternParts) {
			return true
		}
	}
	return false
}

func splitPath(p string) []string {
	p = filepath.Clean(p)
	parts := strings.FieldsFunc(p, func(r rune) bool {
		return r == filepath.Separator || r == '/'
	})
	return parts
}

func matchParts(path, pattern []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}

	if pattern[0] == "**" {
		rest := pattern[1:]
		if len(rest) == 0 {
			return true
		}
		for i := 0; i <= len(path); i++ {
			if matchParts(path[i:], rest) {
				return true
			}
		}
		return false
	}

	if len(path) == 0 {
		return false
	}

	matched, err := filepath.Match(strings.ToLower(pattern[0]), strings.ToLower(path[0]))
	if err != nil || !matched {
		return false
	}

	return matchParts(path[1:], pattern[1:])
}
