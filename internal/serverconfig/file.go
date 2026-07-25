// Package serverconfig loads and saves local server.yaml and .env beside the server binary.
package serverconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	YAMLName = "server.yaml"
	EnvName  = ".env"
)

// File is the on-disk server.yaml schema.
type File struct {
	Host        string   `yaml:"host,omitempty"`
	Port        string   `yaml:"port,omitempty"`
	BotTokens   []string `yaml:"botTokens,omitempty"`
	ChannelID   string   `yaml:"channelId,omitempty"`
	GuildID     string   `yaml:"guildId,omitempty"`
	ChunkSize   int64    `yaml:"chunkSize,omitempty"` // preferred bytes; 0 → tier max when guild set
	PremiumTier int      `yaml:"premiumTier,omitempty"`
	CORS        string   `yaml:"cors,omitempty"`
	APIURL      string   `yaml:"apiUrl,omitempty"`
	LogLevel    string   `yaml:"logLevel,omitempty"`
	DatabaseURL string   `yaml:"databaseUrl,omitempty"`
	ValkeyURL   string   `yaml:"valkeyUrl,omitempty"`
}

// DefaultLocal returns beginner-friendly local defaults. databaseURL should be set by caller.
func DefaultLocal(databaseURL string) File {
	return File{
		Host:        "127.0.0.1",
		Port:        "6373",
		CORS:        "http://localhost:3000",
		APIURL:      "http://127.0.0.1:6373",
		LogLevel:    "info",
		DatabaseURL: databaseURL,
		ValkeyURL:   "",
	}
}

// Dir returns the canonical config directory (…/server/config).
func Dir(installServerDir string) string {
	return filepath.Join(installServerDir, "config")
}

// Path returns the canonical server.yaml path.
func Path(installServerDir string) string {
	return filepath.Join(Dir(installServerDir), YAMLName)
}

// EnvPath returns the canonical .env path.
func EnvPath(installServerDir string) string {
	return filepath.Join(Dir(installServerDir), EnvName)
}

// CurrentDir returns …/server/current.
func CurrentDir(installServerDir string) string {
	return filepath.Join(installServerDir, "current")
}

// Load reads canonical server.yaml. Missing file → empty File, nil error.
func Load(installServerDir string) (File, error) {
	path := Path(installServerDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, nil
		}
		return File{}, err
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("invalid %s: %w", path, err)
	}
	normalize(&f)
	return f, nil
}

// Save writes canonical server.yaml atomically (0600) and syncs presentation into current/.
func Save(installServerDir string, f File) error {
	normalize(&f)
	dir := Dir(installServerDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(&f)
	if err != nil {
		return err
	}
	path := Path(installServerDir)
	if err := atomicWrite(path, data, 0o600); err != nil {
		return err
	}
	return SyncCurrent(installServerDir)
}

// Unset clears named keys in canonical server.yaml.
func Unset(installServerDir string, keys ...string) error {
	f, err := Load(installServerDir)
	if err != nil {
		return err
	}
	for _, k := range keys {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "host":
			f.Host = ""
		case "port":
			f.Port = ""
		case "bottokens", "bot_tokens", "bot-tokens":
			f.BotTokens = nil
		case "channelid", "channel_id", "channel-id":
			f.ChannelID = ""
		case "guildid", "guild_id", "guild-id":
			f.GuildID = ""
		case "chunksize", "chunk_size", "chunk-size":
			f.ChunkSize = 0
		case "premiumtier", "premium_tier", "premium-tier":
			f.PremiumTier = 0
		case "cors":
			f.CORS = ""
		case "apiurl", "api_url", "api-url":
			f.APIURL = ""
		case "loglevel", "log_level", "log-level":
			f.LogLevel = ""
		case "databaseurl", "database_url", "database-url":
			f.DatabaseURL = ""
		case "valkeyurl", "valkey_url", "valkey-url":
			f.ValkeyURL = ""
		default:
			return fmt.Errorf("unknown config key %q", k)
		}
	}
	return Save(installServerDir, f)
}

// EnvMap converts File fields to DISCLOUD_* map (for Sources.YAML).
func EnvMap(f File) map[string]string {
	out := make(map[string]string)
	if f.Host != "" {
		out["DISCLOUD_HOST"] = f.Host
	}
	if f.Port != "" {
		out["DISCLOUD_PORT"] = f.Port
	}
	if tokens := joinTokens(f.BotTokens); tokens != "" {
		out["DISCLOUD_BOT_TOKEN"] = tokens
	}
	if f.ChannelID != "" {
		out["DISCLOUD_CHANNEL_ID"] = f.ChannelID
	}
	if f.GuildID != "" {
		out["DISCLOUD_GUILD_ID"] = f.GuildID
	}
	if f.ChunkSize > 0 {
		out["DISCLOUD_CHUNK_SIZE"] = strconv.FormatInt(f.ChunkSize, 10)
	}
	if f.CORS != "" {
		out["DISCLOUD_CORS"] = f.CORS
	}
	if f.APIURL != "" {
		out["DISCLOUD_API_URL"] = f.APIURL
	}
	if f.LogLevel != "" {
		out["DISCLOUD_LOG_LEVEL"] = f.LogLevel
	}
	if f.DatabaseURL != "" {
		out["DISCLOUD_DATABASE_URL"] = f.DatabaseURL
	}
	// Explicit empty valkey is meaningful for SQLite; only set when non-empty here.
	// Callers that need "empty overrides default" should set the key in DotEnv/Process.
	if f.ValkeyURL != "" {
		out["DISCLOUD_VALKEY_URL"] = f.ValkeyURL
	}
	return out
}

// LoadDotEnv reads canonical .env into a map. Missing → empty map.
func LoadDotEnv(installServerDir string) (map[string]string, error) {
	return ParseDotEnvFile(EnvPath(installServerDir))
}

// MaskSecrets returns a copy of File with bot tokens redacted.
func MaskSecrets(f File) File {
	out := f
	if len(out.BotTokens) > 0 {
		masked := make([]string, len(out.BotTokens))
		for i, t := range out.BotTokens {
			masked[i] = maskToken(t)
		}
		out.BotTokens = masked
	}
	out.DatabaseURL = maskURL(out.DatabaseURL)
	out.ValkeyURL = maskURL(out.ValkeyURL)
	return out
}

func maskToken(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if len(t) <= 8 {
		return "****"
	}
	return t[:4] + "…" + t[len(t)-4:]
}

func maskURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	// Hide userinfo if present.
	if i := strings.Index(u, "://"); i >= 0 {
		rest := u[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 {
			return u[:i+3] + "***@" + rest[at+1:]
		}
	}
	if strings.HasPrefix(strings.ToLower(u), "sqlite:") || strings.HasPrefix(strings.ToLower(u), "file:") {
		return u // path-only, no credentials
	}
	return u
}

// SyncCurrent ensures current/ presents config beside the binary.
// Unix: symlinks to canonical. Windows: atomic copy from canonical.
func SyncCurrent(installServerDir string) error {
	cur := CurrentDir(installServerDir)
	if err := os.MkdirAll(cur, 0o700); err != nil {
		return err
	}
	canonYAML := Path(installServerDir)
	canonEnv := EnvPath(installServerDir)
	curYAML := filepath.Join(cur, YAMLName)
	curEnv := filepath.Join(cur, EnvName)

	if runtime.GOOS == "windows" {
		if _, err := os.Stat(canonYAML); err == nil {
			if err := copyFileAtomic(canonYAML, curYAML, 0o600); err != nil {
				return err
			}
		}
		if _, err := os.Stat(canonEnv); err == nil {
			info, _ := os.Stat(canonEnv)
			mode := os.FileMode(0o600)
			if info != nil {
				mode = info.Mode().Perm()
			}
			if err := copyFileAtomic(canonEnv, curEnv, mode); err != nil {
				return err
			}
		}
		return nil
	}

	// Unix: symlink
	if _, err := os.Stat(canonYAML); err == nil {
		if err := replaceSymlink(curYAML, canonYAML); err != nil {
			return err
		}
	}
	if _, err := os.Stat(canonEnv); err == nil {
		if err := replaceSymlink(curEnv, canonEnv); err != nil {
			return err
		}
	}
	return nil
}

func replaceSymlink(link, target string) error {
	_ = os.Remove(link)
	return os.Symlink(target, link)
}

func copyFileAtomic(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return atomicWrite(dst, data, mode)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func normalize(f *File) {
	f.Host = strings.TrimSpace(f.Host)
	f.Port = strings.TrimSpace(f.Port)
	f.ChannelID = strings.TrimSpace(f.ChannelID)
	f.GuildID = strings.TrimSpace(f.GuildID)
	f.CORS = strings.TrimRight(strings.TrimSpace(f.CORS), "/")
	f.APIURL = strings.TrimRight(strings.TrimSpace(f.APIURL), "/")
	f.LogLevel = strings.ToLower(strings.TrimSpace(f.LogLevel))
	f.DatabaseURL = strings.TrimSpace(f.DatabaseURL)
	f.ValkeyURL = strings.TrimSpace(f.ValkeyURL)
	f.BotTokens = normalizeTokens(f.BotTokens)
	if f.ChunkSize < 0 {
		f.ChunkSize = 0
	}
	if f.PremiumTier < 0 {
		f.PremiumTier = 0
	}
	if f.APIURL == "" && f.Host != "" && f.Port != "" {
		host := f.Host
		if host == "0.0.0.0" || host == "" {
			host = "127.0.0.1"
		}
		f.APIURL = fmt.Sprintf("http://%s:%s", host, f.Port)
	}
}

func normalizeTokens(tokens []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func joinTokens(tokens []string) string {
	tokens = normalizeTokens(tokens)
	return strings.Join(tokens, ",")
}

// UnmarshalYAML decodes server.yaml bytes into File.
func UnmarshalYAML(data []byte, f *File) error {
	if err := yaml.Unmarshal(data, f); err != nil {
		return err
	}
	normalize(f)
	return nil
}

// Get returns a single field value (raw) by key name.
func Get(f File, key string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "host":
		return f.Host, nil
	case "port":
		return f.Port, nil
	case "bottokens", "bot_tokens", "bot-tokens":
		return joinTokens(f.BotTokens), nil
	case "channelid", "channel_id", "channel-id":
		return f.ChannelID, nil
	case "guildid", "guild_id", "guild-id":
		return f.GuildID, nil
	case "chunksize", "chunk_size", "chunk-size":
		if f.ChunkSize <= 0 {
			return "", nil
		}
		return strconv.FormatInt(f.ChunkSize, 10), nil
	case "premiumtier", "premium_tier", "premium-tier":
		if f.PremiumTier <= 0 {
			return "", nil
		}
		return strconv.Itoa(f.PremiumTier), nil
	case "cors":
		return f.CORS, nil
	case "apiurl", "api_url", "api-url":
		return f.APIURL, nil
	case "loglevel", "log_level", "log-level":
		return f.LogLevel, nil
	case "databaseurl", "database_url", "database-url":
		return f.DatabaseURL, nil
	case "valkeyurl", "valkey_url", "valkey-url":
		return f.ValkeyURL, nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}
