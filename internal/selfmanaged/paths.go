package selfmanaged

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// SafeJoin joins destDir with a logical path, rejecting traversal and absolute paths.
func SafeJoin(destDir, logicalPath string) (string, error) {
	raw := strings.ReplaceAll(logicalPath, "\\", "/")
	for _, part := range strings.Split(raw, "/") {
		if part == ".." {
			return "", fmt.Errorf("path traversal rejected")
		}
	}
	logicalPath = path.Clean("/" + raw)
	logicalPath = strings.TrimPrefix(logicalPath, "/")
	if logicalPath == "" || logicalPath == "." {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(logicalPath, "../") || logicalPath == ".." {
		return "", fmt.Errorf("path traversal rejected")
	}
	if filepath.IsAbs(logicalPath) {
		return "", fmt.Errorf("absolute path rejected")
	}
	// Windows drive escape
	if len(logicalPath) >= 2 && logicalPath[1] == ':' {
		return "", fmt.Errorf("drive path rejected")
	}
	full := filepath.Join(destDir, filepath.FromSlash(logicalPath))
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	sep := string(filepath.Separator)
	if absFull != absDest && !strings.HasPrefix(absFull, absDest+sep) {
		return "", fmt.Errorf("path escapes destination")
	}
	return full, nil
}

// BundleExt is the required self-managed bundle filename suffix.
const BundleExt = ".discloud"

// DefaultBundleName returns a default .discloud output name.
func DefaultBundleName(fileCount int, singleBase string) string {
	if fileCount == 1 && singleBase != "" {
		return singleBase + BundleExt
	}
	return fmt.Sprintf("discloud-bundle-%s%s", time.Now().UTC().Format("20060102-150405"), BundleExt)
}

// EnsureDiscloudExt appends or completes a truncated .discloud suffix.
// Returns the fixed path and whether it changed.
func EnsureDiscloudExt(p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", false
	}
	if strings.HasSuffix(strings.ToLower(p), BundleExt) {
		return p, false
	}
	ext := filepath.Ext(p)
	lowExt := strings.ToLower(ext)
	// Incomplete suffix like .dis / .disclou → replace with .discloud.
	if lowExt != "" && lowExt != "." && strings.HasPrefix(BundleExt, lowExt) {
		return p[:len(p)-len(ext)] + BundleExt, true
	}
	return p + BundleExt, true
}

// SelectFiles filters meta.Files by ids or names. empty selectors + all=false → error if multi.
func SelectFiles(files []FileMeta, selectors []string, all bool) ([]FileMeta, error) {
	if all || (len(files) == 1 && len(selectors) == 0) {
		return append([]FileMeta(nil), files...), nil
	}
	if len(selectors) == 0 {
		if len(files) == 0 {
			return nil, fmt.Errorf("bundle has no files")
		}
		return nil, fmt.Errorf("multiple files: pass --file, --all, or select interactively")
	}
	var out []FileMeta
	seen := map[string]struct{}{}
	for _, sel := range selectors {
		sel = strings.TrimSpace(sel)
		found := false
		for _, f := range files {
			if f.ID == sel || f.OriginalName == sel || f.LogicalPath == sel {
				if _, ok := seen[f.ID]; ok {
					found = true
					break
				}
				seen[f.ID] = struct{}{}
				out = append(out, f)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("file not found in bundle: %s", sel)
		}
	}
	return out, nil
}
