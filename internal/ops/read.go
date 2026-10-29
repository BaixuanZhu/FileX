package ops

import (
	"os"

	"filex/internal/encoding"
	"filex/internal/errs"
)

type ReadParams struct {
	Path     string
	Encoding string
	MaxBytes int64
	Offset   int
	Limit    int
}

type ReadResult struct {
	Content    string                `json:"content"`
	Encoding   encoding.DetectionResult `json:"encoding"`
	Size       int64                 `json:"size"`
	Truncated  bool                  `json:"truncated,omitempty"`
}

func Read(p ReadParams) (*ReadResult, *errs.Error) {
	data, err := os.ReadFile(p.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.WithHint(errs.Newf(errs.ENotFound, "file not found: %s", p.Path), "check the path")
		}
		return nil, errs.Newf(errs.EIO, "read failed: %v", err)
	}

	size := int64(len(data))
	truncated := false

	if p.MaxBytes > 0 && int64(len(data)) > p.MaxBytes {
		data = data[:p.MaxBytes]
		truncated = true
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

	content, err := encoding.Decode(data, det.Encoding)
	if err != nil {
		return nil, errs.Newf(errs.EEncoding, "decode failed: %v", err)
	}

	return &ReadResult{
		Content:   content,
		Encoding:  det,
		Size:      size,
		Truncated: truncated,
	}, nil
}
