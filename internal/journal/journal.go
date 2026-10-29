package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Timestamp    string `json:"timestamp"`
	Op           string `json:"op"`
	Path         string `json:"path,omitempty"`
	BackupID     string `json:"backup_id,omitempty"`
	SHA256Before string `json:"sha256_before,omitempty"`
	SHA256After  string `json:"sha256_after,omitempty"`
	Scope        string `json:"scope,omitempty"`
	EnvName      string `json:"env_name,omitempty"`
	EnvValueHash string `json:"env_value_hash,omitempty"`
	ClientInfo   string `json:"client_info,omitempty"`
}

var mu sync.Mutex

func journalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".filex", "journal")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func Record(entry Entry) error {
	mu.Lock()
	defer mu.Unlock()

	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	dir, err := journalDir()
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(dir, "filex.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

func HashEnvValue(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}

func IsSensitiveName(name string) bool {
	upper := strings.ToUpper(name)
	for _, keyword := range []string{
		"PASSWORD", "SECRET", "KEY", "TOKEN", "CREDENTIAL", "PRIVATE", "SIGNATURE",
	} {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

func MaskEnvEntry(op, scope, name, value string) Entry {
	entry := Entry{
		Op:      op,
		Scope:   scope,
		EnvName: name,
	}
	if IsSensitiveName(name) {
		entry.EnvValueHash = HashEnvValue(value)
	}
	return entry
}
