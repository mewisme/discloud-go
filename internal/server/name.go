package server

import (
	"fmt"
	"mime"
	"path/filepath"
	"strings"
)

// formatFileName mirrors the original's kebab-case sanitization: the base
// name becomes lowercase words joined by dashes, the extension is kept.
// ponytail: no accent transliteration ("café" -> "caf", not "cafe"); pulling
// in x/text just for that isn't worth it — upgrade path is x/text/runes.
func formatFileName(name string) string {
	base, ext := name, ""
	if i := strings.LastIndex(name, "."); i > 0 {
		base, ext = name[:i], strings.ToLower(name[i:])
	}
	var b strings.Builder
	lastDash := true // avoid leading dash
	for _, c := range strings.ToLower(base) {
		switch {
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
			b.WriteRune(c)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		out = "file"
	}
	return out + ext
}

// humanBytes renders a byte count for log messages, e.g. "12.34 MB".
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// safeDownloadHeaders picks Content-Type and Content-Disposition for proxied
// downloads. Scriptable types always force attachment + octet-stream.
func safeDownloadHeaders(name string, forceDownload bool) (contentType, disposition string) {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".html", ".htm", ".xhtml", ".svg", ".xml":
		return "application/octet-stream", "attachment"
	}
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		ct = "application/octet-stream"
	}
	disposition = "inline"
	if forceDownload {
		disposition = "attachment"
	}
	// Only allow inline for media/pdf/plain text; everything else attaches.
	switch {
	case strings.HasPrefix(ct, "image/"),
		strings.HasPrefix(ct, "video/"),
		strings.HasPrefix(ct, "audio/"),
		ct == "application/pdf",
		ct == "text/plain":
		return ct, disposition
	default:
		return "application/octet-stream", "attachment"
	}
}
