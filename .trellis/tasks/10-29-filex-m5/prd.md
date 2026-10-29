# M5: large file handling (tail, read paging, grep stream)

## Goal

Implement tail (encoding-aware, --follow), read --offset/--limit (line-level paging), grep --stream (line-by-line decoding). edit rejects >100MB with E_TOO_LARGE.

## Requirements

- `tail`: scan from file end, encoding-aware line breaks, --lines=N (default 10, max 1000), --follow (context-cancelable), global line numbers
- `read --offset --limit`: line-level paging, encoding-aware, global line number prefix
- `grep --stream`: line-by-line decode + match, --max=N, reject binary files
- `edit`: reject >100MB files with E_TOO_LARGE + hint
- Performance: 100MB tail <1s, grep --stream memory <50MB

## Acceptance Criteria

- [ ] tail --lines=10 on 100MB file returns in <1s
- [ ] grep --stream on 100MB file uses <50MB memory
- [ ] read --offset=1000000 --limit=10 returns correct content
- [ ] edit on >100MB file returns E_TOO_LARGE
- [ ] go build/test/vet pass

## Notes

- tail scans backward from EOF in chunks, counts newlines
- --follow uses polling with context cancellation
- grep --stream uses bufio.Scanner with increased buffer for long lines
- read paging reuses existing read + adds line offset/limit
