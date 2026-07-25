package localserver

import (
	"context"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// tailWindow bounds how much of the log is read to satisfy a tail request.
const tailWindow = 256 << 10

var (
	// keyed secrets: DISCLOUD_BOT_TOKEN=…, "token": "…", authorization: Bearer …
	secretKeyRe = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*(?:token|secret|password|authorization|api[_-]?key)[A-Za-z0-9_.-]*)("?\s*[:=]\s*"?)([^\s",}]+)`)
	// bare Discord bot tokens
	discordTokenRe = regexp.MustCompile(`[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{20,}`)
)

// Tail returns the last n lines of the server log. Missing log → no lines.
func Tail(p Paths, n int) ([]string, error) {
	if n <= 0 {
		n = 50
	}
	f, err := os.Open(p.LogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := int64(0)
	if info.Size() > tailWindow {
		offset = info.Size() - tailWindow
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if offset > 0 && len(lines) > 1 {
		lines = lines[1:] // drop the partial first line
	}
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// RedactedTail is Tail with secrets masked, for embedding in error messages.
func RedactedTail(p Paths, n int) string {
	lines, err := Tail(p, n)
	if err != nil {
		return "(log unavailable: " + err.Error() + ")"
	}
	for i, line := range lines {
		lines[i] = Redact(line)
	}
	return strings.Join(lines, "\n")
}

// Follow writes the last n lines then streams appended lines until ctx ends.
func Follow(ctx context.Context, p Paths, n int, w io.Writer) error {
	lines, err := Tail(p, n)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
	}

	var offset int64
	if info, err := os.Stat(p.LogPath()); err == nil {
		offset = info.Size()
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
		info, err := os.Stat(p.LogPath())
		if err != nil {
			continue // log rotated away; wait for it to come back
		}
		if info.Size() < offset {
			offset = 0 // truncated
		}
		if info.Size() == offset {
			continue
		}
		f, err := os.Open(p.LogPath())
		if err != nil {
			continue
		}
		if _, err := f.Seek(offset, io.SeekStart); err == nil {
			written, cerr := io.Copy(w, f)
			offset += written
			if cerr != nil {
				f.Close()
				return cerr
			}
		}
		f.Close()
	}
}

// Redact masks bot tokens and keyed secrets in a log line.
func Redact(line string) string {
	line = secretKeyRe.ReplaceAllString(line, "${1}${2}***")
	return discordTokenRe.ReplaceAllString(line, "***")
}
