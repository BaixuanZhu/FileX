package ops

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"

	"filex/internal/encoding"
	"filex/internal/errs"
)

const (
	tailChunkSize = 8192
	tailMaxLines  = 1000
)

type TailParams struct {
	Path   string
	Lines  int
	Follow bool
}

type TailLine struct {
	Number  int    `json:"number"`
	Content string `json:"content"`
}

type TailResult struct {
	Lines     []TailLine               `json:"lines"`
	Encoding  encoding.DetectionResult `json:"encoding"`
	TotalSize int64                    `json:"total_size"`
	Follow    bool                     `json:"follow,omitempty"`
}

func Tail(p TailParams) (*TailResult, *errs.Error) {
	if p.Lines <= 0 {
		p.Lines = 10
	}
	if p.Lines > tailMaxLines {
		return nil, errs.Newf(errs.EUsage, "--lines must be between 1 and %d", tailMaxLines)
	}

	if pErr := checkPolicy(p.Path); pErr != nil {
		return nil, pErr
	}

	f, err := os.Open(p.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.Newf(errs.ENotFound, "file not found: %s", p.Path)
		}
		return nil, errs.Newf(errs.EIO, "open failed: %v", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, errs.Newf(errs.EIO, "stat failed: %v", err)
	}

	fileSize := info.Size()
	if fileSize == 0 {
		return &TailResult{Lines: nil, TotalSize: 0}, nil
	}

	var header []byte
	if fileSize > 512 {
		header = make([]byte, 512)
		f.Read(header)
	} else {
		header = make([]byte, fileSize)
		f.Read(header)
	}
	det := encoding.Detect(header)
	if det.Encoding == encoding.EncBinary {
		return nil, errs.WithHint(
			errs.New(errs.EEncoding, "file appears to be binary"),
			"use --encoding to force a specific encoding",
		)
	}

	byteOffsets, totalLines, err := findTailOffsets(f, fileSize, p.Lines, det.Encoding)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "tail scan failed: %v", err)
	}

	lines, err := readTailLines(f, byteOffsets, det.Encoding, p.Lines, totalLines)
	if err != nil {
		return nil, errs.Newf(errs.EIO, "tail read failed: %v", err)
	}

	result := &TailResult{
		Lines:     lines,
		Encoding:  det,
		TotalSize: fileSize,
	}

	if p.Follow {
		result.Follow = true
		followTail(p.Path, det.Encoding, &result.Lines)
	}

	return result, nil
}

func findTailOffsets(f *os.File, fileSize int64, maxLines int, enc encoding.Encoding) ([]int64, int, error) {
	pos := fileSize
	var newlines []int64
	buf := make([]byte, tailChunkSize)

	// Collect maxLines+1 newlines to determine where to start reading
	for pos > 0 && len(newlines) < maxLines+1 {
		chunkSize := int64(tailChunkSize)
		if pos < chunkSize {
			chunkSize = pos
		}
		pos -= chunkSize

		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return nil, 0, err
		}
		n, err := f.Read(buf[:chunkSize])
		if err != nil && err != io.EOF {
			return nil, 0, err
		}

		chunk := buf[:n]
		if enc == encoding.EncUTF16LE || enc == encoding.EncUTF16BE {
			chunk = countUTF16Newlines(chunk, enc == encoding.EncUTF16LE)
		}

		for i := len(chunk) - 1; i >= 0; i-- {
			if chunk[i] == '\n' {
				absOffset := pos + int64(i)
				newlines = append(newlines, absOffset)
				if len(newlines) >= maxLines+1 {
					break
				}
			}
		}
	}

	totalLines := countTotalLines(f, fileSize, enc)
	return newlines, totalLines, nil
}

func countUTF16Newlines(chunk []byte, littleEndian bool) []byte {
	var result []byte
	step := 2
	for i := 0; i+1 < len(chunk); i += step {
		var b byte
		if littleEndian {
			if chunk[i] == '\n' && chunk[i+1] == 0 {
				b = '\n'
			}
		} else {
			if chunk[i] == 0 && chunk[i+1] == '\n' {
				b = '\n'
			}
		}
		if b != 0 {
			result = append(result, b)
		} else {
			result = append(result, ' ')
		}
	}
	return result
}

func countTotalLines(f *os.File, fileSize int64, enc encoding.Encoding) int {
	f.Seek(0, io.SeekStart)
	count := 0
	buf := make([]byte, tailChunkSize)

	for {
		n, err := f.Read(buf)
		if n == 0 {
			break
		}
		if enc == encoding.EncUTF16LE || enc == encoding.EncUTF16BE {
			for i := 0; i+1 < n; i += 2 {
				if enc == encoding.EncUTF16LE {
					if buf[i] == '\n' && buf[i+1] == 0 {
						count++
					}
				} else {
					if buf[i] == 0 && buf[i+1] == '\n' {
						count++
					}
				}
			}
		} else {
			count += bytes.Count(buf[:n], []byte{'\n'})
		}
		if err != nil {
			break
		}
	}
	f.Seek(0, io.SeekStart)
	return count
}

func readTailLines(f *os.File, offsets []int64, enc encoding.Encoding, maxLines int, totalLines int) ([]TailLine, error) {
	// Determine where to start reading
	var firstOffset int64
	var startLine int
	if len(offsets) > maxLines {
		// We have more newlines than requested lines; read from the (maxLines)th newline
		firstOffset = offsets[maxLines] + 1
		startLine = totalLines - maxLines + 1
	} else {
		// We have all (or fewer) newlines; read from beginning
		firstOffset = 0
		startLine = 1
	}

	if len(offsets) == 0 {
		firstOffset = 0
		startLine = 1
	}

	f.Seek(firstOffset, io.SeekStart)
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	content, err := encoding.Decode(data, enc)
	if err != nil {
		return nil, err
	}

	content = strings.TrimRight(content, "\r\n")
	if content == "" {
		return nil, nil
	}
	rawLines := strings.Split(content, "\n")

	var lines []TailLine
	lineNum := startLine
	for _, raw := range rawLines {
		raw = strings.TrimRight(raw, "\r")
		lines = append(lines, TailLine{Number: lineNum, Content: raw})
		lineNum++
	}

	return lines, nil
}

func followTail(path string, enc encoding.Encoding, result *[]TailLine) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	info, _ := f.Stat()
	pos := info.Size()
	lineNum := 0
	if len(*result) > 0 {
		lineNum = (*result)[len(*result)-1].Number
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cur, err := os.Open(path)
			if err != nil {
				return
			}
			curInfo, err := cur.Stat()
			if err != nil {
				cur.Close()
				return
			}
			if curInfo.Size() <= pos {
				cur.Close()
				continue
			}
			cur.Seek(pos, io.SeekStart)
			data, _ := io.ReadAll(cur)
			cur.Close()

			content, err := encoding.Decode(data, enc)
			if err != nil {
				return
			}
			content = strings.TrimRight(content, "\r\n")
			newLines := strings.Split(content, "\n")
			for _, raw := range newLines {
				raw = strings.TrimRight(raw, "\r")
				if raw != "" {
					lineNum++
					*result = append(*result, TailLine{Number: lineNum, Content: raw})
				}
			}
			pos = curInfo.Size()
		}
	}
}
