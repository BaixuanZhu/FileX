package ops

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"filex/internal/backup"
	"filex/internal/encoding"
	"filex/internal/errs"
	"filex/internal/guard"
	"filex/internal/winfs"
)

type StatResult struct {
	Path     string                    `json:"path"`
	Size     int64                     `json:"size"`
	Mode     string                    `json:"mode"`
	ModTime  time.Time                 `json:"mod_time"`
	IsDir    bool                      `json:"is_dir"`
	Encoding *encoding.DetectionResult `json:"encoding,omitempty"`
	Lock     *winfs.LockStatus         `json:"lock,omitempty"`
	SHA256   string                    `json:"sha256,omitempty"`
}

func Stat(path string) (*StatResult, *errs.Error) {
	if pErr := checkPolicy(path); pErr != nil {
		return nil, pErr
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "resolve path failed: %v", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.Newf(errs.ENotFound, "not found: %s", path)
		}
		return nil, errs.Newf(errs.EIO, "stat failed: %v", err)
	}

	result := &StatResult{
		Path:    absPath,
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}

	if !info.IsDir() {
		lock, err := winfs.CheckLock(absPath)
		if err == nil {
			result.Lock = &lock
		}

		if info.Size() > 0 && info.Size() < 10*1024*1024 {
			data, err := os.ReadFile(absPath)
			if err == nil {
				det := encoding.Detect(data)
				result.Encoding = &det

				h := sha256.Sum256(data)
				result.SHA256 = hex.EncodeToString(h[:])
			}
		}
	}

	return result, nil
}

type LockResult = winfs.LockStatus

func Lock(path string) (*LockResult, *errs.Error) {
	if pErr := checkPolicy(path); pErr != nil {
		return nil, pErr
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "resolve path failed: %v", err)
	}

	if !winfs.FileExists(absPath) {
		return nil, errs.Newf(errs.ENotFound, "not found: %s", path)
	}

	status, err := winfs.CheckLock(absPath)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "lock check failed: %v", err)
	}

	return &status, nil
}

type LsEntry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	IsDir   bool      `json:"is_dir"`
	ModTime time.Time `json:"mod_time"`
}

func Ls(path string, depth int) ([]LsEntry, *errs.Error) {
	if pErr := checkPolicy(path); pErr != nil {
		return nil, pErr
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "resolve path failed: %v", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.Newf(errs.ENotFound, "not found: %s", path)
		}
		return nil, errs.Newf(errs.EIO, "stat failed: %v", err)
	}

	if !info.IsDir() {
		return nil, errs.Newf(errs.EUsage, "not a directory: %s", path)
	}

	var entries []LsEntry
	err = lsRecursive(absPath, depth, 0, &entries)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "list failed: %v", err)
	}

	return entries, nil
}

func lsRecursive(dir string, maxDepth, currentDepth int, entries *[]LsEntry) error {
	if maxDepth > 0 && currentDepth >= maxDepth {
		return nil
	}

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, e := range dirEntries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		entry := LsEntry{
			Name:    e.Name(),
			Path:    filepath.Join(dir, e.Name()),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime(),
		}
		*entries = append(*entries, entry)

		if e.IsDir() {
			lsRecursive(filepath.Join(dir, e.Name()), maxDepth, currentDepth+1, entries)
		}
	}

	return nil
}

type HashResult struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func Hash(path string) (*HashResult, *errs.Error) {
	if pErr := checkPolicy(path); pErr != nil {
		return nil, pErr
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "resolve path failed: %v", err)
	}

	hash, err := backup.FileSHA256(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.Newf(errs.ENotFound, "not found: %s", path)
		}
		return nil, errs.Newf(errs.EIO, "hash failed: %v", err)
	}

	info, _ := os.Stat(absPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}

	return &HashResult{
		Path:   absPath,
		SHA256: hash,
		Size:   size,
	}, nil
}

type WriteParams struct {
	Path          string
	Content       string
	Encoding      string
	CreateParents bool
	ExpectSHA256  string
	Backup        bool
}

type WriteResult struct {
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	SHA256Before string `json:"sha256_before,omitempty"`
	SHA256After  string `json:"sha256_after"`
	Backup       string `json:"backup,omitempty"`
	Created      bool   `json:"created"`
}

func Write(p WriteParams) (*WriteResult, *errs.Error) {
	if pErr := checkPolicy(p.Path); pErr != nil {
		return nil, pErr
	}

	absPath, err := filepath.Abs(p.Path)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "resolve path failed: %v", err)
	}

	r, gErr := gate(guard.Action{Verb: guard.VerbWrite, Path: absPath})
	if gErr != nil {
		return nil, gErr
	}

	result := &WriteResult{Path: absPath}

	existed := winfs.FileExists(absPath)
	if existed {
		hashBefore, _ := backup.FileSHA256(absPath)
		result.SHA256Before = hashBefore

		if p.ExpectSHA256 != "" && p.ExpectSHA256 != hashBefore {
			return nil, errs.WithHint(
				errs.WithRetryable(
					errs.Newf(errs.EConflict, "SHA-256 mismatch: expected %s, got %s", p.ExpectSHA256, hashBefore),
					true,
				),
				"file was modified since you last read it; re-read and retry",
			)
		}

		if p.Backup {
			entry, err := backup.CreateBackup(absPath)
			if err != nil {
				return nil, errs.Newf(errs.EIO, "backup failed: %v", err)
			}
			result.Backup = entry.ID
		}
	} else {
		result.Created = true
		if p.CreateParents {
			dir := filepath.Dir(absPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, errs.Newf(errs.EIO, "create parents failed: %v", err)
			}
		}
	}

	enc := encoding.EncUTF8
	if p.Encoding != "" && p.Encoding != "auto" {
		enc = encoding.ParseEncoding(p.Encoding)
		if enc == encoding.EncUnknown {
			return nil, errs.Newf(errs.EEncoding, "unknown encoding: %s", p.Encoding)
		}
	}

	data, err := encoding.Encode(p.Content, enc)
	if err != nil {
		return nil, errs.Newf(errs.EEncoding, "encode failed: %v", err)
	}

	// Atomic write: create temp file in same directory, write, then MoveFileEx.
	// This ensures that a crash mid-write leaves the original file intact.
	dir := filepath.Dir(absPath)
	tmpPath := filepath.Join(dir, fmt.Sprintf(".filex-tmp-%d", time.Now().UnixNano()))

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		os.Remove(tmpPath)
		return nil, errs.Newf(errs.EIO, "write to temp file failed: %v", err)
	}

	if err := winfs.AtomicReplace(tmpPath, absPath); err != nil {
		os.Remove(tmpPath)
		return nil, errs.Newf(errs.EIO, "atomic replace failed: %v", err)
	}

	hashAfter, _ := backup.FileSHA256(absPath)
	result.SHA256After = hashAfter
	result.Size = int64(len(data))

	recordExecuted("write", absPath, result.Backup, result.SHA256Before, result.SHA256After, "", r)

	return result, nil
}

type RmParams struct {
	Path   string
	Purge  bool
	DryRun bool
}

type RmResult struct {
	Path    string `json:"path"`
	Deleted bool   `json:"deleted"`
	Purged  bool   `json:"purged"`
	Trash   string `json:"trash,omitempty"`

	// Dry-run only: what would happen, without touching the filesystem.
	DryRun     bool   `json:"dry_run,omitempty"`
	Action     string `json:"action,omitempty"`
	WillMoveTo string `json:"will_move_to,omitempty"`
}

func trashRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".filex", "trash"), nil
}

func previewRm(absPath string, purge bool) *RmResult {
	result := &RmResult{Path: absPath, DryRun: true}
	if purge {
		result.Action = "permanent_delete"
		return result
	}
	result.Action = "move_to_trash"
	if root, err := trashRoot(); err == nil {
		result.WillMoveTo = root
	}
	return result
}

func Rm(p RmParams) (*RmResult, *errs.Error) {
	if pErr := checkPolicy(p.Path); pErr != nil {
		return nil, pErr
	}

	absPath, err := filepath.Abs(p.Path)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "resolve path failed: %v", err)
	}

	if !winfs.FileExists(absPath) {
		return nil, errs.Newf(errs.ENotFound, "not found: %s", p.Path)
	}

	if p.DryRun {
		return previewRm(absPath, p.Purge), nil
	}

	r, gErr := gate(guard.Action{Verb: guard.VerbRm, Path: absPath, Purge: p.Purge})
	if gErr != nil {
		return nil, gErr
	}

	result := &RmResult{Path: absPath}

	if p.Purge {
		if err := os.RemoveAll(absPath); err != nil {
			return nil, errs.Newf(errs.EIO, "delete failed: %v", err)
		}
		result.Deleted = true
		result.Purged = true
		recordExecuted("rm", absPath, "", "", "", "purge", r)
		return result, nil
	}

	trashDir, err := trashRoot()
	if err != nil {
		return nil, errs.Newf(errs.EIO, "resolve trash dir failed: %v", err)
	}
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		return nil, errs.Newf(errs.EIO, "create trash dir failed: %v", err)
	}

	ts := time.Now().Format("20060102-150405")
	trashPath := filepath.Join(trashDir, fmt.Sprintf("%s-%s", filepath.Base(absPath), ts))

	if err := os.Rename(absPath, trashPath); err != nil {
		data, readErr := os.ReadFile(absPath)
		if readErr != nil {
			return nil, errs.Newf(errs.EIO, "move to trash failed: %v", err)
		}
		if writeErr := os.WriteFile(trashPath, data, 0644); writeErr != nil {
			return nil, errs.Newf(errs.EIO, "move to trash failed: %v", err)
		}
		if delErr := os.RemoveAll(absPath); delErr != nil {
			return nil, errs.Newf(errs.EIO, "copy succeeded but original delete failed: %v", delErr)
		}
	}

	result.Deleted = true
	result.Trash = trashPath

	recordExecuted("rm", absPath, "", "", "", "trash:"+trashPath, r)

	return result, nil
}
