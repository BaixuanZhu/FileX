package mcpserver

type Profile string

const (
	ProfileRO Profile = "ro"
	ProfileRW Profile = "rw"
)

var readOnlyTools = map[string]bool{
	"read":    true,
	"grep":    true,
	"stat":    true,
	"lock":    true,
	"ls":      true,
	"tail":    true,
	"backup":  true,
	"env":     true,
}

var readOnlyEnvActions = map[string]bool{
	"get":  true,
	"list": true,
}

var readOnlyBackupActions = map[string]bool{
	"list": true,
}

func (p Profile) String() string {
	return string(p)
}

func (p Profile) AllowsTool(name string) bool {
	if p == ProfileRW {
		return true
	}
	return readOnlyTools[name]
}

func (p Profile) AllowsEnvAction(action string) bool {
	if p == ProfileRW {
		return true
	}
	return readOnlyEnvActions[action]
}

func (p Profile) AllowsBackupAction(action string) bool {
	if p == ProfileRW {
		return true
	}
	return readOnlyBackupActions[action]
}

func ParseProfile(s string) (Profile, bool) {
	switch s {
	case "ro":
		return ProfileRO, true
	case "rw":
		return ProfileRW, true
	default:
		return "", false
	}
}
