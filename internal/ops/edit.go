package ops

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"filex/internal/encoding"
	"filex/internal/errs"
)

type EditParams struct {
	Path         string
	Find         string
	Replace      string
	All          bool
	DryRun       bool
	ExpectSHA256 string
	Encoding     string
	Backup       bool
}

type EditResult struct {
	DryRun       bool                   `json:"dry_run"`
	Encoding    encoding.DetectionResult `json:"encoding"`
	SHA256Before string               `json:"sha256_before,omitempty"`
	SHA256After  string               `json:"sha256_after,omitempty"`
	encoding.EditResult
}

func Edit(p EditParams) (*EditResult, *errs.Error) {
	if pErr := checkPolicy(p.Path); pErr != nil {
		return nil, pErr
	}

	info, err := os.Stat(p.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.WithHint(errs.Newf(errs.ENotFound, "file not found: %s", p.Path), "check the path")
		}
		return nil, errs.Newf(errs.EIO, "stat failed: %v", err)
	}
	if info.Size() > 100*1024*1024 {
		return nil, errs.WithHint(
			errs.Newf(errs.ETooLarge, "file is %dMB, edit limit is 100MB", info.Size()/(1024*1024)),
			"use grep --stream to locate, tail to read context, then edit a smaller region",
		)
	}

	data, err := os.ReadFile(p.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.WithHint(errs.Newf(errs.ENotFound, "file not found: %s", p.Path), "check the path")
		}
		return nil, errs.Newf(errs.EIO, "read failed: %v", err)
	}

	hashBefore := sha256.Sum256(data)
	result := &EditResult{
		SHA256Before: hex.EncodeToString(hashBefore[:]),
		DryRun:       p.DryRun,
	}

	var det encoding.DetectionResult
	if p.Encoding != "" && p.Encoding != "auto" {
		enc := encoding.ParseEncoding(p.Encoding)
		if enc == encoding.EncUnknown {
			return nil, errs.Newf(errs.EEncoding, "unknown encoding: %s", p.Encoding)
		}
		det = encoding.DetectionResult{Encoding: enc, Confidence: encoding.ConfExact}
	} else {
		det = encoding.Detect(data)
		if det.Encoding == encoding.EncBinary {
			return nil, errs.WithHint(
				errs.New(errs.EEncoding, "file appears to be binary"),
				"use --encoding to force a specific encoding",
			)
		}
	}
	result.Encoding = det

	if p.ExpectSHA256 != "" {
		if p.ExpectSHA256 != result.SHA256Before {
			return nil, errs.WithHint(
				errs.WithRetryable(errs.Newf(errs.EConflict, "SHA-256 mismatch: expected %s, got %s", p.ExpectSHA256, result.SHA256Before), true),
				"file was modified since you last read it; re-read and retry",
			)
		}
	}

	newData, editResult, err := encoding.Edit(data, p.Find, p.Replace, det.Encoding, p.All)
	if err != nil {
		if err == encoding.ErrNotFound {
			return nil, errs.Newf(errs.ENotFound, "pattern not found: %q", p.Find)
		}
		if err == encoding.ErrNotUnique {
			return nil, errs.WithHint(
				errs.Newf(errs.EConflict, "pattern found %d times; use --all to replace all", editResult.Matches),
				"use --all to replace all occurrences, or make the pattern more specific",
			)
		}
		return nil, errs.Newf(errs.EEncoding, "edit failed: %v", err)
	}

	result.EditResult = editResult

	if p.DryRun {
		return result, nil
	}

	hashAfter := sha256.Sum256(newData)
	result.SHA256After = hex.EncodeToString(hashAfter[:])

	if err := os.WriteFile(p.Path, newData, 0644); err != nil {
		return nil, errs.Newf(errs.EIO, "write failed: %v", err)
	}

	absPath, _ := filepath.Abs(p.Path)
	recordJournal("edit", absPath, "", result.SHA256Before, result.SHA256After)

	return result, nil
}
