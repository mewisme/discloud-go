package localserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
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
// Each line is passed through FormatLogLine for human display.
func Follow(ctx context.Context, p Paths, n int, w io.Writer) error {
	lines, err := Tail(p, n)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if err := writeLogLine(w, line); err != nil {
			return err
		}
	}

	var offset int64
	if info, err := os.Stat(p.LogPath()); err == nil {
		offset = info.Size()
	}
	var carry string
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Stat(p.LogPath())
		if err != nil {
			continue // log rotated away; wait for it to come back
		}
		if info.Size() < offset {
			offset = 0 // truncated
			carry = ""
		}
		if info.Size() == offset {
			continue
		}
		chunk, err := readFrom(p.LogPath(), offset)
		if err != nil {
			continue
		}
		offset += int64(len(chunk))
		text := carry + string(chunk)
		parts := strings.Split(text, "\n")
		carry = parts[len(parts)-1]
		for _, line := range parts[:len(parts)-1] {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := writeLogLine(w, line); err != nil {
				return err
			}
		}
	}
}

func readFrom(path string, offset int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}

func writeLogLine(w io.Writer, line string) error {
	out := FormatLogLine(line)
	if out == "" {
		return nil
	}
	_, err := fmt.Fprintln(w, out)
	return err
}

// FormatLogLine turns a slog JSON log line into a compact human line.
// Non-JSON lines are redacted and returned as-is.
// Example: 15:04:05  INFO   request  method=GET path=/readyz status=200
func FormatLogLine(line string) string {
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return ""
	}
	redacted := Redact(line)
	var m map[string]any
	if err := json.Unmarshal([]byte(redacted), &m); err != nil {
		return redacted
	}

	msg, _ := m["msg"].(string)
	if msg == "" {
		if s, ok := m["message"].(string); ok {
			msg = s
		}
	}
	level := "INFO"
	if v, ok := m["level"]; ok && v != nil {
		if s := strings.ToUpper(strings.TrimSpace(fmt.Sprint(v))); s != "" && s != "<NIL>" {
			level = s
		}
	}

	tstr := formatLogTime(m["time"])

	skip := map[string]struct{}{
		"time": {}, "level": {}, "msg": {}, "message": {}, "source": {},
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if _, ok := skip[k]; ok {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	if tstr != "" {
		b.WriteString(tstr)
		b.WriteString("  ")
	}
	b.WriteString(fmt.Sprintf("%-5s", level))
	if msg != "" {
		b.WriteString("  ")
		b.WriteString(msg)
	}
	for _, k := range keys {
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(formatLogAttr(m[k]))
	}
	return b.String()
}

func formatLogTime(v any) string {
	switch t := v.(type) {
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
			if parsed, err := time.Parse(layout, t); err == nil {
				return parsed.Local().Format("15:04:05")
			}
		}
		return t
	case float64:
		// JSON numbers are rare for slog time; treat as unix seconds if huge.
		if t > 1e9 {
			return time.Unix(int64(t), 0).Local().Format("15:04:05")
		}
	}
	return ""
}

func formatLogAttr(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		if strings.ContainsAny(x, " \t\"=") {
			return strconvQuote(x)
		}
		return x
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case bool:
		return fmt.Sprintf("%t", x)
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(b)
	}
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Redact masks bot tokens and keyed secrets in a log line.
func Redact(line string) string {
	line = secretKeyRe.ReplaceAllString(line, "${1}${2}***")
	return discordTokenRe.ReplaceAllString(line, "***")
}
