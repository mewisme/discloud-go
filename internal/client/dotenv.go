package client

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const dotenvMaxParents = 8

// loadDotEnvValues finds a .env near the working directory and returns CLI
// settings. Prefer DISCLOUD_BASE / DISCLOUD_ORIGIN; fall back to API_URL /
// WEB_ORIGIN (same keys the server/compose .env already uses).
func loadDotEnvValues() (base, origin string) {
	path := findDotEnv(dotenvMaxParents)
	if path == "" {
		return "", ""
	}
	m, err := parseDotEnvFile(path)
	if err != nil {
		return "", ""
	}
	base = firstNonEmpty(m["DISCLOUD_BASE"], m["API_URL"])
	origin = firstNonEmpty(m["DISCLOUD_ORIGIN"], m["WEB_ORIGIN"])
	return base, origin
}

func findDotEnv(maxParents int) string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i <= maxParents; i++ {
		p := filepath.Join(dir, ".env")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// parseDotEnvFile reads KEY=VALUE lines. Only unquoted / double-quoted values;
// comments and blank lines are skipped. Does not expand variables.
func parseDotEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		out[key] = val
	}
	return out, sc.Err()
}
