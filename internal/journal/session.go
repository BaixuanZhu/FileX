package journal

import (
	"crypto/rand"
	"encoding/hex"
	"os"
)

// Where a record's session id came from. These are the only three answers; a
// fourth would mean guessing, and a guessed session key makes a meaningless
// distribution look like a conclusion.
const (
	SessionSourceMCPConnection = "mcp_connection"
	SessionSourceEnv           = "env"
	// SessionSourceAbsent labels an unlabelled record. Such records leave the
	// field empty rather than writing "absent": the column is omitempty, so an
	// audit line from before this feature and one from a run with no session stay
	// byte-identical. That compatibility is worth more than saying "absent" out
	// loud, and stats reads an empty source as absent anyway.
	SessionSourceAbsent = "absent"
)

const (
	maxSessionIDLen = 64
	envSessionVar   = "FILEX_SESSION_ID"
)

// NewSessionID returns an id for one MCP connection.
//
// It is an observability label, not a credential: nothing in filex authorises
// anything with it, so forging one only pollutes the forger's own statistics.
func NewSessionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// SessionFromEnv reads the id an orchestrator set for the work it is doing.
// The value is used exactly as given or not at all: an unusable value leaves the
// record unlabelled rather than inventing a session.
func SessionFromEnv() (id, source string) {
	value := os.Getenv(envSessionVar)
	if !ValidSessionID(value) {
		return "", ""
	}
	return value, SessionSourceEnv
}

// ValidSessionID keeps anything but a plain identifier out of the audit log. An
// id is echoed into terminal output and into JSON, so a newline or an embedded
// quote in it would let whoever set the variable write a forged record.
func ValidSessionID(id string) bool {
	if id == "" || len(id) > maxSessionIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
