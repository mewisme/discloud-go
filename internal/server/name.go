package server

import (
	"fmt"
	"mime"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// formatFileName sanitizes a file name or relative path for storage.
// Basenames are kebab-cased; path segments keep structure with "/" separators.
// Rejects ".." , absolute paths, NUL, and overly deep/long paths by falling
// back to a safe basename when invalid.
//
// ponytail: no accent transliteration ("café" -> "caf", not "cafe"); pulling
// in x/text just for that isn't worth it — upgrade path is x/text/runes.
func formatFileName(name string) string {
	name = strings.ReplaceAll(name, `\`, `/`)
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsRune(name, 0) {
		return "file"
	}
	if strings.HasPrefix(name, "/") || strings.Contains(name, ":") && len(name) > 1 && name[1] == ':' {
		// Absolute / Windows drive — use basename only.
		name = path.Base(strings.ReplaceAll(name, `\`, `/`))
	}

	parts := strings.Split(name, "/")
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." {
			continue
		}
		if p == ".." {
			return sanitizeBase(path.Base(name))
		}
		clean = append(clean, sanitizeSegment(p))
	}
	if len(clean) == 0 {
		return "file"
	}
	if len(clean) > maxPathDepth {
		clean = clean[len(clean)-maxPathDepth:]
	}
	out := strings.Join(clean, "/")
	if len(out) > maxFileNameLen {
		out = out[len(out)-maxFileNameLen:]
		if i := strings.Index(out, "/"); i >= 0 {
			out = out[i+1:]
		}
	}
	if out == "" {
		return "file"
	}
	return out
}

func sanitizeSegment(seg string) string {
	base, ext := seg, ""
	if i := strings.LastIndex(seg, "."); i > 0 {
		base, ext = seg[:i], strings.ToLower(seg[i:])
	}
	var b strings.Builder
	lastDash := true
	for _, c := range strings.ToLower(base) {
		switch {
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
			b.WriteRune(c)
			lastDash = false
		case unicode.IsLetter(c) || unicode.IsDigit(c):
			// Keep non-ASCII letters as-is (lowercased where possible).
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

func sanitizeBase(name string) string {
	return sanitizeSegment(path.Base(strings.ReplaceAll(name, `\`, `/`)))
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
