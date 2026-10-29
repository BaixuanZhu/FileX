package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filex/internal/errs"
)

const (
	DefaultRetention = 3
	backupDirName    = ".filex/backups"
)

type BackupEntry struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	File      string    `json:"file"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

func backupDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, backupDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func pathHash(absPath string) string {
	h := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(h[:16])
}

func CreateBackup(srcPath string) (*BackupEntry, error) {
	absPath, err := filepath.Abs(srcPath)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	dir, err := backupDir()
	if err != nil {
		return nil, err
	}

	ph := pathHash(absPath)
	now := time.Now()
	ts := now.Unix()
	filename := fmt.Sprintf("%s-%d.bak", ph, ts)
	filePath := filepath.Join(dir, filename)

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return nil, err
	}

	entry := &BackupEntry{
		ID:        fmt.Sprintf("%s-%d", ph, ts),
		Path:      absPath,
		File:      filePath,
		Size:      int64(len(data)),
		SHA256:    hashStr,
		CreatedAt: now,
	}

	metaPath := filePath + ".json"
	meta, _ := json.MarshalIndent(entry, "", "  ")
	os.WriteFile(metaPath, meta, 0644)

	return entry, nil
}

func ListBackups(filterPath string) ([]BackupEntry, error) {
	dir, err := backupDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var backups []BackupEntry
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		var entry BackupEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}

		if filterPath != "" {
			absFilter, _ := filepath.Abs(filterPath)
			if !strings.EqualFold(entry.Path, absFilter) {
				continue
			}
		}

		backups = append(backups, entry)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

func RestoreBackup(id string) (*BackupEntry, error) {
	backups, err := ListBackups("")
	if err != nil {
		return nil, err
	}

	var target *BackupEntry
	for i := range backups {
		if backups[i].ID == id {
			target = &backups[i]
			break
		}
	}
	if target == nil {
		return nil, errs.Newf(errs.ENotFound, "backup not found: %s", id)
	}

	if !fileExists(target.File) {
		return nil, errs.Newf(errs.EIO, "backup file missing: %s", target.File)
	}

	if fileExists(target.Path) {
		if _, err := CreateBackup(target.Path); err != nil {
			return nil, errs.Newf(errs.EIO, "pre-restore backup failed: %v", err)
		}
	}

	data, err := os.ReadFile(target.File)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(target.Path, data, 0644); err != nil {
		return nil, err
	}

	return target, nil
}

func PruneBackups(retention int) (int, error) {
	if retention <= 0 {
		retention = DefaultRetention
	}

	backups, err := ListBackups("")
	if err != nil {
		return 0, err
	}

	byPath := make(map[string][]BackupEntry)
	for _, b := range backups {
		byPath[b.Path] = append(byPath[b.Path], b)
	}

	pruned := 0
	for _, entries := range byPath {
		if len(entries) <= retention {
			continue
		}
		toRemove := entries[retention:]
		for _, e := range toRemove {
			os.Remove(e.File)
			os.Remove(e.File + ".json")
			pruned++
		}
	}

	return pruned, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

var ErrBackupNotFound = errors.New("backup not found")
