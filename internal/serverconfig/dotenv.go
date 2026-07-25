package serverconfig

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// ParseDotEnvFile parses a .env file into KEY→value. Missing file → empty map.
func ParseDotEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer f.Close()

	out := make(map[string]string)
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		key, val, skip, err := parseDotEnvLine(sc.Text())
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		if skip {
			continue
		}
		out[key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseDotEnvLine(line string) (key, val string, skip bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", true, nil
	}
	if strings.HasPrefix(line, "export ") {
		line = strings.TrimSpace(line[len("export "):])
	}
	eq := strings.IndexByte(line, '=')
	if eq <= 0 {
		return "", "", false, fmt.Errorf("expected KEY=value")
	}
	key = strings.TrimSpace(line[:eq])
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false, fmt.Errorf("invalid key")
	}
	raw := strings.TrimSpace(line[eq+1:])
	val, err = unquoteEnv(raw)
	return key, val, false, err
}

func unquoteEnv(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if raw[0] != '"' && raw[0] != '\'' {
		if i := strings.Index(raw, " #"); i >= 0 {
			raw = strings.TrimSpace(raw[:i])
		}
		return raw, nil
	}
	quote, _ := utf8.DecodeRuneInString(raw)
	var b strings.Builder
	escaped := false
	i := utf8.RuneLen(quote)
	for i < len(raw) {
		r, size := utf8.DecodeRuneInString(raw[i:])
		i += size
		if escaped {
			switch r {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteRune(r)
			}
			escaped = false
			continue
		}
		if r == '\\' && quote == '"' {
			escaped = true
			continue
		}
		if r == quote {
			rest := strings.TrimSpace(raw[i:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return "", fmt.Errorf("trailing junk after quoted value")
			}
			return b.String(), nil
		}
		b.WriteRune(r)
	}
	return "", fmt.Errorf("unterminated quote")
}
