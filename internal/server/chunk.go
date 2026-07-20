package server

import (
	"errors"
	"io"
)

// forEachChunk reads r in pieces of at most chunkSize bytes and calls fn for
// each piece with its zero-based index. Each piece is freshly allocated and
// owned by fn, so fn may retain it (e.g. hand it to a goroutine). Returns the
// total number of bytes read.
func forEachChunk(r io.Reader, chunkSize int, fn func(idx int, data []byte) error) (int64, error) {
	var total int64
	for idx := 0; ; idx++ {
		buf := make([]byte, chunkSize)
		n, err := io.ReadFull(r, buf)
		total += int64(n)
		if n > 0 {
			if err := fn(idx, buf[:n]); err != nil {
				return total, err
			}
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

// byteRange is an inclusive byte range within a file.
type byteRange struct {
	start, end int64
}

var errInvalidRange = errors.New("invalid range")

// parseRange parses a single-range "bytes=..." header against a file of the
// given size. maxWindow caps open-ended ranges ("bytes=start-") so seeking in
// large media doesn't stream the whole tail; explicit ranges are honored as-is.
func parseRange(header string, size, maxWindow int64) (byteRange, error) {
	const prefix = "bytes="
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return byteRange{}, errInvalidRange
	}
	spec := header[len(prefix):]
	dash := -1
	for i, c := range spec {
		if c == '-' {
			dash = i
			break
		}
	}
	if dash < 0 {
		return byteRange{}, errInvalidRange
	}
	startStr, endStr := spec[:dash], spec[dash+1:]

	if startStr == "" { // suffix range: last N bytes
		n, err := parseInt(endStr)
		if err != nil || n <= 0 {
			return byteRange{}, errInvalidRange
		}
		if n > size {
			n = size
		}
		return byteRange{start: size - n, end: size - 1}, nil
	}

	start, err := parseInt(startStr)
	if err != nil || start >= size {
		return byteRange{}, errInvalidRange
	}
	if endStr == "" { // open-ended: cap the window
		end := start + maxWindow
		if end > size-1 {
			end = size - 1
		}
		return byteRange{start: start, end: end}, nil
	}
	end, err := parseInt(endStr)
	if err != nil || end < start {
		return byteRange{}, errInvalidRange
	}
	if end > size-1 {
		end = size - 1
	}
	return byteRange{start: start, end: end}, nil
}

func parseInt(s string) (int64, error) {
	if s == "" {
		return 0, errInvalidRange
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalidRange
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// partSpan describes which bytes to read from one stored chunk.
type partSpan struct {
	idx        int   // chunk index
	start, end int64 // inclusive byte offsets within the chunk
}

// partsForRange maps a byte range of the whole file onto the chunks that hold
// it, with per-chunk offsets.
func partsForRange(chunkSize int64, r byteRange) []partSpan {
	first := r.start / chunkSize
	last := r.end / chunkSize
	spans := make([]partSpan, 0, last-first+1)
	for i := first; i <= last; i++ {
		s := partSpan{idx: int(i), start: 0, end: chunkSize - 1}
		if i == first {
			s.start = r.start % chunkSize
		}
		if i == last {
			s.end = r.end % chunkSize
		}
		spans = append(spans, s)
	}
	return spans
}
