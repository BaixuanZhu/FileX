package winenv

import (
	"regexp"
	"strings"
)

type Vars map[string]string

var envVarPattern = regexp.MustCompile(`(?i)%([A-Z0-9_]+)%`)

const maxExpandDepth = 5

func ExpandEnv(s string, vars Vars) string {
	return expandString(s, vars, 0)
}

func expandString(s string, vars Vars, depth int) string {
	if depth > maxExpandDepth {
		return s
	}
	return envVarPattern.ReplaceAllStringFunc(s, func(m string) string {
		name := strings.ToUpper(m[1 : len(m)-1])
		if v, ok := vars[name]; ok {
			return expandString(v, vars, depth+1)
		}
		return m
	})
}

func ComposeVars(system, user map[string]string) Vars {
	merged := make(Vars, len(system)+len(user))
	for k, v := range system {
		merged[strings.ToUpper(k)] = v
	}
	for k, v := range user {
		merged[strings.ToUpper(k)] = v
	}
	for round := 0; round < maxExpandDepth; round++ {
		changed := false
		for k, v := range merged {
			if nv := expandString(v, merged, 1); nv != v {
				merged[k] = nv
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return merged
}
