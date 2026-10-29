package encoding

import (
	"bytes"
	"errors"
)

var (
	ErrNotFound     = errors.New("pattern not found")
	ErrNotUnique    = errors.New("pattern not unique (use --all)")
	ErrEncodeFailed = errors.New("encode failed")
)

type EditResult struct {
	Modified    bool `json:"modified"`
	Matches     int  `json:"matches"`
	BytesBefore int  `json:"bytes_before"`
	BytesAfter  int  `json:"bytes_after"`
}

func Edit(data []byte, find, replace string, enc Encoding, all bool) ([]byte, EditResult, error) {
	findBytes, err := Encode(find, enc)
	if err != nil {
		return nil, EditResult{}, ErrEncodeFailed
	}
	replaceBytes, err := Encode(replace, enc)
	if err != nil {
		return nil, EditResult{}, ErrEncodeFailed
	}

	matches := bytes.Count(data, findBytes)
	if matches == 0 {
		return data, EditResult{Modified: false, Matches: 0, BytesBefore: len(data), BytesAfter: len(data)}, ErrNotFound
	}
	if matches > 1 && !all {
		return data, EditResult{Modified: false, Matches: matches, BytesBefore: len(data), BytesAfter: len(data)}, ErrNotUnique
	}

	var result []byte
	if all {
		result = bytes.ReplaceAll(data, findBytes, replaceBytes)
	} else {
		result = bytes.Replace(data, findBytes, replaceBytes, 1)
	}

	return result, EditResult{
		Modified:    true,
		Matches:     matches,
		BytesBefore: len(data),
		BytesAfter:  len(result),
	}, nil
}

func FindAll(data []byte, pattern string, enc Encoding) ([]int, error) {
	patternBytes, err := Encode(pattern, enc)
	if err != nil {
		return nil, ErrEncodeFailed
	}

	var positions []int
	idx := 0
	for {
		pos := bytes.Index(data[idx:], patternBytes)
		if pos == -1 {
			break
		}
		positions = append(positions, idx+pos)
		idx += pos + len(patternBytes)
	}
	return positions, nil
}
