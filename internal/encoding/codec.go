package encoding

import (
	"errors"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	ErrInvalidEncoding = errors.New("invalid encoding")
	ErrDecodeFailed    = errors.New("decode failed")
)

func Decode(data []byte, enc Encoding) (string, error) {
	switch enc {
	case EncUTF8, EncUTF8BOM:
		return decodeUTF8(data, enc == EncUTF8BOM)
	case EncUTF16LE:
		return decodeUTF16(data, true)
	case EncUTF16BE:
		return decodeUTF16(data, false)
	case EncGBK, EncGB18030:
		return decodeGB18030(data)
	default:
		return "", ErrInvalidEncoding
	}
}

func Encode(s string, enc Encoding) ([]byte, error) {
	switch enc {
	case EncUTF8:
		return []byte(s), nil
	case EncUTF8BOM:
		return append(BOMUTF8, []byte(s)...), nil
	case EncUTF16LE:
		return encodeUTF16(s, true)
	case EncUTF16BE:
		return encodeUTF16(s, false)
	case EncGBK, EncGB18030:
		return encodeGB18030(s)
	default:
		return nil, ErrInvalidEncoding
	}
}

func decodeUTF8(data []byte, hasBOM bool) (string, error) {
	raw := data
	if hasBOM && len(data) >= 3 {
		raw = data[3:]
	}
	if !utf8.Valid(raw) {
		return "", ErrDecodeFailed
	}
	return string(raw), nil
}

func decodeUTF16(data []byte, littleEndian bool) (string, error) {
	if len(data)%2 != 0 {
		return "", ErrDecodeFailed
	}

	u16s := make([]uint16, len(data)/2)
	for i := 0; i < len(data); i += 2 {
		if littleEndian {
			u16s[i/2] = uint16(data[i]) | uint16(data[i+1])<<8
		} else {
			u16s[i/2] = uint16(data[i])<<8 | uint16(data[i+1])
		}
	}

	runes := utf16.Decode(u16s)
	// Strip BOM (U+FEFF) if present — BOM is a file-level marker, not content.
	if len(runes) > 0 && runes[0] == '\uFEFF' {
		runes = runes[1:]
	}
	return string(runes), nil
}

func encodeUTF16(s string, littleEndian bool) ([]byte, error) {
	runes := []rune(s)
	u16s := utf16.Encode(runes)

	// Note: Encode does NOT add a BOM. BOM is a file-level marker, not part
	// of the text content. The Edit operation uses Encode to produce byte
	// patterns for searching within raw file data, where a BOM would break
	// matching. File-level BOM handling belongs in the ops layer (write/edit).
	data := make([]byte, len(u16s)*2)
	for i, u := range u16s {
		if littleEndian {
			data[i*2] = byte(u)
			data[i*2+1] = byte(u >> 8)
		} else {
			data[i*2] = byte(u >> 8)
			data[i*2+1] = byte(u)
		}
	}
	return data, nil
}

func decodeGB18030(data []byte) (string, error) {
	var result []rune
	i := 0
	for i < len(data) {
		b := data[i]
		if b < 0x80 {
			result = append(result, rune(b))
			i++
			continue
		}
		if i+1 >= len(data) {
			return "", ErrDecodeFailed
		}
		b2 := data[i+1]
		if b2 >= 0x40 && b2 != 0x7F && b2 != 0xFF {
			code := uint16(b)*256 + uint16(b2)
			r := gb18030ToRune(code)
			if r == 0 {
				return "", ErrDecodeFailed
			}
			result = append(result, r)
			i += 2
		} else if b2 >= 0x30 && b2 <= 0x39 {
			if i+3 >= len(data) {
				return "", ErrDecodeFailed
			}
			b3 := data[i+2]
			b4 := data[i+3]
			// GB18030 four-byte decoding:
			// code = (b1-0x81)*12600 + (b2-0x30)*1260 + (b3-0x81)*10 + (b4-0x30)
			// where 12600 = 10*126*10 and 1260 = 126*10
			code := uint32(b-0x81)*12600 + uint32(b2-0x30)*1260 + uint32(b3-0x81)*10 + uint32(b4-0x30)
			r := gb18030FourToRune(code)
			if r == 0 {
				return "", ErrDecodeFailed
			}
			result = append(result, r)
			i += 4
		} else {
			return "", ErrDecodeFailed
		}
	}
	return string(result), nil
}

func encodeGB18030(s string) ([]byte, error) {
	var result []byte
	for _, r := range s {
		if r < 0x80 {
			result = append(result, byte(r))
			continue
		}
		code := runeToGB18030(r)
		if code == 0 {
			return nil, ErrDecodeFailed
		}
		if code < 0x10000 {
			result = append(result, byte(code>>8), byte(code))
		} else {
			result = append(result, byte(code>>24), byte(code>>16), byte(code>>8), byte(code))
		}
	}
	return result, nil
}

// gb18030ToRune maps a 2-byte GB18030 code to a Unicode rune.
// NOTE (v0.1 limitation): This uses a simplified identity-style mapping rather
// than the full GB18030 ↔ Unicode lookup table. It works for the common CJK
// Unified Ideographs block (which occupies a contiguous range in GB18030's
// two-byte zone) but will not correctly handle less common characters that
// require the full mapping table. A future version should use the official
// GB18030 mapping data from the Unicode Consortium.
func gb18030ToRune(code uint16) rune {
	if code >= 0x8140 && code <= 0xFE4F {
		return rune(code)
	}
	return 0
}

// gb18030FourToRune maps a 4-byte GB18030 code to a Unicode rune.
// NOTE (v0.1 limitation): simplified mapping — see gb18030ToRune comment.
func gb18030FourToRune(code uint32) rune {
	return rune(code)
}

func runeToGB18030(r rune) uint32 {
	if r < 0x80 {
		return uint32(r)
	}
	if r >= 0x8140 && r <= 0xFE4F {
		return uint32(r)
	}
	return 0
}
