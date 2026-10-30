package ops

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"

	"filex/internal/encoding"
	"filex/internal/errs"
)

const (
	grepMaxDefault      = 100
	grepMaxBuffer       = 10 * 1024 * 1024
	grepStreamThreshold = 10 * 1024 * 1024
)

type GrepParams struct {
	Pattern string
	Paths   []string
	Max     int
	Stream  bool
}

type GrepMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

type GrepResult struct {
	Matches        []GrepMatch `json:"matches"`
	TotalMatched   int         `json:"total_matched"`
	Truncated      bool        `json:"truncated,omitempty"`
	TotalEstimated int         `json:"total_estimated,omitempty"`
}

func Grep(p GrepParams) (*GrepResult, *errs.Error) {
	if p.Pattern == "" {
		return nil, errs.New(errs.EUsage, "pattern is required")
	}
	if p.Max <= 0 {
		p.Max = grepMaxDefault
	}

	re, err := regexp.Compile(p.Pattern)
	if err != nil {
		return nil, errs.Newf(errs.EUsage, "invalid pattern: %v", err)
	}

	result := &GrepResult{}

	for _, path := range p.Paths {
		if pErr := checkPolicy(path); pErr != nil {
			continue
		}

		matches, err := grepFile(path, re, p.Max-len(result.Matches), p.Stream)
		if err != nil {
			continue
		}
		result.Matches = append(result.Matches, matches...)
		result.TotalMatched += len(matches)

		if len(result.Matches) >= p.Max {
			result.Truncated = true
			break
		}
	}

	return result, nil
}

func grepFile(path string, re *regexp.Regexp, remaining int, stream bool) ([]GrepMatch, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if !stream && info.Size() > grepStreamThreshold {
		return grepFileStream(path, f, re, remaining)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	det := encoding.Detect(data)
	if det.Encoding == encoding.EncBinary {
		return nil, encoding.ErrDecodeFailed
	}

	content, err := encoding.Decode(data, det.Encoding)
	if err != nil {
		return nil, err
	}

	var matches []GrepMatch
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		if re.MatchString(line) {
			matches = append(matches, GrepMatch{
				Path:    path,
				Line:    i + 1,
				Content: line,
			})
			if len(matches) >= remaining {
				break
			}
		}
	}

	return matches, nil
}

func grepFileStream(path string, f *os.File, re *regexp.Regexp, remaining int) ([]GrepMatch, error) {
	header := make([]byte, 512)
	n, _ := f.Read(header)
	det := encoding.Detect(header[:n])
	if det.Encoding == encoding.EncBinary {
		return nil, encoding.ErrDecodeFailed
	}

	f.Seek(0, io.SeekStart)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, grepMaxBuffer), grepMaxBuffer)
	scanner.Split(scanLinesEncoding(det.Encoding))

	var matches []GrepMatch
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, GrepMatch{
				Path:    path,
				Line:    lineNum,
				Content: line,
			})
			if len(matches) >= remaining {
				break
			}
		}
	}

	return matches, nil
}

func scanLinesEncoding(enc encoding.Encoding) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}

		switch enc {
		case encoding.EncUTF16LE:
			return scanUTF16Lines(data, atEOF, true)
		case encoding.EncUTF16BE:
			return scanUTF16Lines(data, atEOF, false)
		default:
			return scanUTF8Lines(data, atEOF)
		}
	}
}

func scanUTF8Lines(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		line := data[:i]
		line = bytes.TrimRight(line, "\r")
		return i + 1, line, nil
	}
	if atEOF {
		line := bytes.TrimRight(data, "\r")
		if len(line) > 0 {
			return len(data), line, nil
		}
		return len(data), nil, nil
	}
	return 0, nil, nil
}

func scanUTF16Lines(data []byte, atEOF bool, littleEndian bool) (int, []byte, error) {
	step := 2
	for i := 0; i+1 < len(data); i += step {
		isNL := false
		if littleEndian {
			isNL = data[i] == '\n' && data[i+1] == 0
		} else {
			isNL = data[i] == 0 && data[i+1] == '\n'
		}
		if isNL {
			line := decodeUTF16Chunk(data[:i], littleEndian)
			return i + step, []byte(line), nil
		}
	}
	if atEOF && len(data) >= step {
		line := decodeUTF16Chunk(data, littleEndian)
		if len(strings.TrimSpace(line)) > 0 {
			return len(data), []byte(line), nil
		}
		return len(data), nil, nil
	}
	return 0, nil, nil
}

func decodeUTF16Chunk(data []byte, littleEndian bool) string {
	var runes []rune
	for i := 0; i+1 < len(data); i += 2 {
		var r rune
		if littleEndian {
			r = rune(uint16(data[i]) | uint16(data[i+1])<<8)
		} else {
			r = rune(uint16(data[i])<<8 | uint16(data[i+1]))
		}
		runes = append(runes, r)
	}
	return string(runes)
}
