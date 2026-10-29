package encoding

import (
	"bytes"
	"errors"
	"unicode/utf8"
)

type Encoding string

const (
	EncUTF8      Encoding = "utf8"
	EncUTF8BOM   Encoding = "utf8bom"
	EncUTF16LE   Encoding = "utf16le"
	EncUTF16BE   Encoding = "utf16be"
	EncUTF32LE   Encoding = "utf32le"
	EncUTF32BE   Encoding = "utf32be"
	EncGBK       Encoding = "gbk"
	EncGB18030   Encoding = "gb18030"
	EncBinary    Encoding = "binary"
	EncUnknown   Encoding = "unknown"
)

type Confidence string

const (
	ConfExact    Confidence = "exact"
	ConfProbable Confidence = "probable"
)

var (
	BOMUTF8    = []byte{0xEF, 0xBB, 0xBF}
	BOMUTF16LE = []byte{0xFF, 0xFE}
	BOMUTF16BE = []byte{0xFE, 0xFF}
	BOMUTF32LE = []byte{0xFF, 0xFE, 0x00, 0x00}
	BOMUTF32BE = []byte{0x00, 0x00, 0xFE, 0xFF}
)

type DetectionResult struct {
	Encoding   Encoding   `json:"encoding"`
	BOM        bool       `json:"bom"`
	Confidence Confidence `json:"confidence"`
}

var ErrBinary = errors.New("file appears to be binary")

func Detect(data []byte) DetectionResult {
	if len(data) == 0 {
		return DetectionResult{Encoding: EncUTF8, BOM: false, Confidence: ConfExact}
	}

	if r := detectBOM(data); r.Encoding != EncUnknown {
		return r
	}

	if utf8.Valid(data) {
		return DetectionResult{Encoding: EncUTF8, BOM: false, Confidence: ConfExact}
	}

	if canDecodeGB18030(data) {
		return DetectionResult{Encoding: EncGB18030, BOM: false, Confidence: ConfProbable}
	}

	return DetectionResult{Encoding: EncBinary, BOM: false, Confidence: ConfProbable}
}

func detectBOM(data []byte) DetectionResult {
	if len(data) >= 4 {
		if bytes.HasPrefix(data, BOMUTF32LE) {
			return DetectionResult{Encoding: EncUTF32LE, BOM: true, Confidence: ConfExact}
		}
		if bytes.HasPrefix(data, BOMUTF32BE) {
			return DetectionResult{Encoding: EncUTF32BE, BOM: true, Confidence: ConfExact}
		}
	}
	if len(data) >= 3 {
		if bytes.HasPrefix(data, BOMUTF8) {
			return DetectionResult{Encoding: EncUTF8BOM, BOM: true, Confidence: ConfExact}
		}
	}
	if len(data) >= 2 {
		if bytes.HasPrefix(data, BOMUTF16LE) {
			return DetectionResult{Encoding: EncUTF16LE, BOM: true, Confidence: ConfExact}
		}
		if bytes.HasPrefix(data, BOMUTF16BE) {
			return DetectionResult{Encoding: EncUTF16BE, BOM: true, Confidence: ConfExact}
		}
	}
	return DetectionResult{Encoding: EncUnknown, BOM: false, Confidence: ConfExact}
}

func canDecodeGB18030(data []byte) bool {
	i := 0
	for i < len(data) {
		b := data[i]
		if b < 0x80 {
			i++
			continue
		}
		if b == 0x80 || b == 0xFF {
			return false
		}
		if i+1 >= len(data) {
			return false
		}
		b2 := data[i+1]
		if b2 < 0x40 || b2 == 0x7F || b2 == 0xFF {
			if b2 >= 0x30 && b2 <= 0x39 {
				if i+3 >= len(data) {
					return false
				}
				b3 := data[i+2]
				b4 := data[i+3]
				if b3 < 0x81 || b3 > 0xFE || b4 < 0x30 || b4 > 0x39 {
					return false
				}
				i += 4
				continue
			}
			return false
		}
		i += 2
	}
	return true
}

func ParseEncoding(s string) Encoding {
	switch s {
	case "utf8":
		return EncUTF8
	case "utf8bom":
		return EncUTF8BOM
	case "utf16le":
		return EncUTF16LE
	case "utf16be":
		return EncUTF16BE
	case "utf32le":
		return EncUTF32LE
	case "utf32be":
		return EncUTF32BE
	case "gbk":
		return EncGBK
	case "gb18030":
		return EncGB18030
	default:
		return EncUnknown
	}
}
