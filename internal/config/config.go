// Package config loads and validates DisCloud server configuration.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AppSecretFile is the cwd-relative app secret (Docker WORKDIR /data → volume).
const AppSecretFile = ".app.secret"

// VisitorHashSaltFile is the cwd-relative salt file (Docker WORKDIR /data → volume).
const VisitorHashSaltFile = ".visitor.secret"

// visitorHashSaltFileLegacy is the pre-rename salt filename; migrated once to VisitorHashSaltFile.
const visitorHashSaltFileLegacy = ".visitor_hash_salt"

const secretBytes = 32

// ErrInvalid is returned for missing or malformed environment variables.
// The API process exits 0 on this error so Compose restart:on-failure does not loop.
var ErrInvalid = errors.New("invalid configuration")

// Backend identifies the metadata store.
type Backend string

const (
	BackendPostgres Backend = "postgres"
	BackendSQLite   Backend = "sqlite"
)

// Sources holds configuration layers. Merge precedence is Process > DotEnv > YAML > Defaults.
type Sources struct {
	Defaults map[string]string
	YAML     map[string]string
	DotEnv   map[string]string
	Process  map[string]string
}

// Merge returns a deterministic map without mutating the process environment.
func Merge(s Sources) map[string]string {
	out := make(map[string]string)
	for _, layer := range []map[string]string{s.Defaults, s.YAML, s.DotEnv, s.Process} {
		for k, v := range layer {
			if k == "" {
				continue
			}
			out[k] = v
		}
	}
	return out
}

// ProcessEnvMap snapshots selected DISCLOUD_* keys from the process environment.
func ProcessEnvMap() map[string]string {
	keys := []string{
		"DISCLOUD_HOST", "DISCLOUD_PORT", "DISCLOUD_DATABASE_URL", "DISCLOUD_VALKEY_URL",
		"DISCLOUD_BOT_TOKEN", "DISCLOUD_CHANNEL_ID", "DISCLOUD_GUILD_ID", "DISCLOUD_CHUNK_SIZE",
		"DISCLOUD_API_URL", "DISCLOUD_APP_SECRET",
		"DISCLOUD_CORS", "DISCLOUD_TRUST_PROXY", "DISCLOUD_CAPTCHA_SECRET",
		"DISCLOUD_RL_UPLOAD", "DISCLOUD_RL_DOWNLOAD", "DISCLOUD_MAX_USER_BYTES",
		"DISCLOUD_MAX_ANON_UPLOADS", "DISCLOUD_MAX_ANON_BYTES", "DISCLOUD_MAX_RAW_UPLOAD",
		"DISCLOUD_LOG_LEVEL", "DISCLOUD_DATA_DIR", "DISCLOUD_CONFIG_DIR",
	}
	out := make(map[string]string)
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			out[k] = v
		}
	}
	return out
}

// DefaultsMap returns built-in default DISCLOUD_* values.
func DefaultsMap() map[string]string {
	return map[string]string{
		"DISCLOUD_PORT":             "6373",
		"DISCLOUD_LOG_LEVEL":        "info",
		"DISCLOUD_RL_UPLOAD":        "60",
		"DISCLOUD_RL_DOWNLOAD":      "120",
		"DISCLOUD_MAX_USER_BYTES":   "0",
		"DISCLOUD_MAX_ANON_UPLOADS": "50",
		"DISCLOUD_MAX_ANON_BYTES":   strconv.FormatInt(2<<30, 10),
		"DISCLOUD_MAX_RAW_UPLOAD":   "0",
	}
}

type Config struct {
	Host        string
	Port        string
	DatabaseURL string
	ValkeyURL   string
	Backend     Backend
	// DiscordBotToken is the raw env value: one token, or comma-separated
	// tokens that divide uploads across bots.
	DiscordBotToken  string
	DiscordChannelID string
	// DiscordGuildID is optional; empty → legacy 8 MiB chunk size.
	DiscordGuildID string
	// PreferredChunkSize is optional preferred upload chunk bytes (0 → tier max when guild set).
	PreferredChunkSize int64
	// APIURL is used to build share links. When empty, links are
	// derived from the incoming request's Host and forwarded proto.
	APIURL string
	// AppSecret is the root secret for HMAC key derivation (sessions, file tokens).
	AppSecret string
	// WebOrigin is the exact browser origin allowed for CORS credentials
	// (from DISCLOUD_CORS, e.g. http://localhost:3000).
	WebOrigin string
	// CookieSecure is true when WebOrigin uses HTTPS.
	CookieSecure bool
	// TrustProxy honors X-Forwarded-For / X-Real-IP when a trusted edge strips client values.
	TrustProxy bool
	// DataDir is the persistent data directory. Empty means cwd-relative secrets (Docker).
	DataDir string
	// LogLevel is debug|info|warn|error.
	LogLevel string

	// Abuse protection (Phase 4). Zero rate limits disable that limiter.
	RateLimitUploadPerMin   int
	RateLimitDownloadPerMin int
	MaxUserBytes            int64
	MaxAnonUploadsPerDay    int
	MaxAnonBytesPerDay      int64
	MaxRawUploadBytes       int64
	CaptchaSecret           string
}

// ListenAddr returns the TCP listen address using Host and Port.
func (c Config) ListenAddr() string {
	return net.JoinHostPort(c.Host, c.Port)
}

// AppSecretPath returns the path used for the app secret file.
func (c Config) AppSecretPath() string {
	if c.DataDir != "" {
		return filepath.Join(c.DataDir, AppSecretFile)
	}
	return AppSecretFile
}

// VisitorSecretPath returns the path used for the visitor salt file.
func (c Config) VisitorSecretPath() string {
	if c.DataDir != "" {
		return filepath.Join(c.DataDir, VisitorHashSaltFile)
	}
	return VisitorHashSaltFile
}

// SlogLevel maps LogLevel to slog.Level.
func (c Config) SlogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// CacheBackendName returns "valkey" or "memory" for status display.
func (c Config) CacheBackendName() string {
	if strings.TrimSpace(c.ValkeyURL) == "" {
		return "memory"
	}
	return "valkey"
}

// StoreBackendName returns "postgres" or "sqlite" for status display.
func (c Config) StoreBackendName() string {
	return string(c.Backend)
}

// Load reads and validates environment variables (process env only).
func Load() (Config, error) {
	return Decode(Merge(Sources{
		Defaults: DefaultsMap(),
		Process:  ProcessEnvMap(),
	}))
}

// LoadFromSources merges sources and decodes a Config.
func LoadFromSources(s Sources) (Config, error) {
	if s.Defaults == nil {
		s.Defaults = DefaultsMap()
	}
	return Decode(Merge(s))
}

// Decode builds a validated Config from a merged DISCLOUD_* map.
func Decode(env map[string]string) (Config, error) {
	get := func(k string) string { return strings.TrimSpace(env[k]) }
	c := Config{
		Host:             get("DISCLOUD_HOST"),
		Port:             get("DISCLOUD_PORT"),
		DatabaseURL:      get("DISCLOUD_DATABASE_URL"),
		ValkeyURL:        get("DISCLOUD_VALKEY_URL"),
		DiscordBotToken:  get("DISCLOUD_BOT_TOKEN"),
		DiscordChannelID: get("DISCLOUD_CHANNEL_ID"),
		DiscordGuildID:   get("DISCLOUD_GUILD_ID"),
		APIURL:           get("DISCLOUD_API_URL"),
		AppSecret:        env["DISCLOUD_APP_SECRET"], // may keep spaces intentionally
		WebOrigin:        get("DISCLOUD_CORS"),
		TrustProxy:       truthy(get("DISCLOUD_TRUST_PROXY")),
		CaptchaSecret:    get("DISCLOUD_CAPTCHA_SECRET"),
		DataDir:          get("DISCLOUD_DATA_DIR"),
		LogLevel:         strings.ToLower(get("DISCLOUD_LOG_LEVEL")),
	}
	if c.Port == "" {
		c.Port = "6373"
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}

	var missing []string
	for _, kv := range []struct{ name, v string }{
		{"DISCLOUD_DATABASE_URL", c.DatabaseURL},
		{"DISCLOUD_BOT_TOKEN", c.DiscordBotToken},
		{"DISCLOUD_CHANNEL_ID", c.DiscordChannelID},
		{"DISCLOUD_CORS", c.WebOrigin},
	} {
		if kv.v == "" {
			missing = append(missing, kv.name)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("%w: missing required environment variable(s): %s", ErrInvalid, strings.Join(missing, ", "))
	}

	backend, err := ParseDatabaseURL(c.DatabaseURL)
	if err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	c.Backend = backend.Backend

	if c.Backend == BackendPostgres && c.ValkeyURL == "" {
		return Config{}, fmt.Errorf("%w: missing required environment variable(s): DISCLOUD_VALKEY_URL", ErrInvalid)
	}
	if c.ValkeyURL != "" {
		if err := validateURLScheme("DISCLOUD_VALKEY_URL", c.ValkeyURL, "valkey", "redis"); err != nil {
			return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}

	if err := validateLogLevel(c.LogLevel); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := validatePort(c.Port); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	var nErr error
	if c.PreferredChunkSize, nErr = parseBytes(get("DISCLOUD_CHUNK_SIZE"), 0); nErr != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, nErr)
	}
	if c.RateLimitUploadPerMin, nErr = parseInt(get("DISCLOUD_RL_UPLOAD"), 60); nErr != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, nErr)
	}
	if c.RateLimitDownloadPerMin, nErr = parseInt(get("DISCLOUD_RL_DOWNLOAD"), 120); nErr != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, nErr)
	}
	if c.MaxUserBytes, nErr = parseBytes(get("DISCLOUD_MAX_USER_BYTES"), 0); nErr != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, nErr)
	}
	if c.MaxAnonUploadsPerDay, nErr = parseInt(get("DISCLOUD_MAX_ANON_UPLOADS"), 50); nErr != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, nErr)
	}
	if c.MaxAnonBytesPerDay, nErr = parseBytes(get("DISCLOUD_MAX_ANON_BYTES"), 2<<30); nErr != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, nErr)
	}
	if c.MaxRawUploadBytes, nErr = parseBytes(get("DISCLOUD_MAX_RAW_UPLOAD"), 0); nErr != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, nErr)
	}

	if c.APIURL != "" {
		origin, _, err := parseOrigin("DISCLOUD_API_URL", c.APIURL)
		if err != nil {
			return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		c.APIURL = origin
	}

	if c.AppSecret == "" {
		s, err := loadOrCreateSecret(c.AppSecretPath(), secretBytes)
		if err != nil {
			return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		c.AppSecret = s
	}
	if len(c.AppSecret) < 32 {
		return Config{}, fmt.Errorf("%w: DISCLOUD_APP_SECRET must be at least 32 characters", ErrInvalid)
	}
	if !hasToken(c.DiscordBotToken) {
		return Config{}, fmt.Errorf("%w: DISCLOUD_BOT_TOKEN has no usable tokens", ErrInvalid)
	}
	origin, secure, err := parseOrigin("DISCLOUD_CORS", c.WebOrigin)
	if err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	c.WebOrigin = origin
	c.CookieSecure = secure
	return c, nil
}

// DatabaseInfo is the result of ParseDatabaseURL.
type DatabaseInfo struct {
	Backend Backend
	// Path is set for SQLite (filesystem path). Empty for Postgres.
	Path string
	// DriverDSN is the string passed to database/sql for SQLite, or the PG URL for Postgres.
	DriverDSN string
	Raw       string
}

// ParseDatabaseURL parses postgres://, postgresql://, sqlite:, and file: URLs.
func ParseDatabaseURL(raw string) (DatabaseInfo, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DatabaseInfo{}, fmt.Errorf("DISCLOUD_DATABASE_URL is empty")
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "postgres://"), strings.HasPrefix(lower, "postgresql://"):
		if err := validateURLScheme("DISCLOUD_DATABASE_URL", raw, "postgres", "postgresql"); err != nil {
			return DatabaseInfo{}, err
		}
		return DatabaseInfo{Backend: BackendPostgres, DriverDSN: raw, Raw: raw}, nil
	case strings.HasPrefix(lower, "sqlite:"):
		path := raw[len("sqlite:"):]
		path = strings.TrimPrefix(path, "//")
		path = sqlitePathFromRaw(path)
		if path == "" {
			return DatabaseInfo{}, fmt.Errorf("DISCLOUD_DATABASE_URL sqlite path is empty")
		}
		return DatabaseInfo{
			Backend:   BackendSQLite,
			Path:      path,
			DriverDSN: sqliteDriverDSN(path),
			Raw:       raw,
		}, nil
	case strings.HasPrefix(lower, "file:"):
		path := raw[len("file:"):]
		path = strings.TrimPrefix(path, "//")
		// Strip query for path extraction.
		if i := strings.IndexAny(path, "?#"); i >= 0 {
			path = path[:i]
		}
		path = sqlitePathFromRaw(path)
		if path == "" {
			return DatabaseInfo{}, fmt.Errorf("DISCLOUD_DATABASE_URL file path is empty")
		}
		return DatabaseInfo{
			Backend:   BackendSQLite,
			Path:      path,
			DriverDSN: sqliteDriverDSN(path),
			Raw:       raw,
		}, nil
	default:
		return DatabaseInfo{}, fmt.Errorf("DISCLOUD_DATABASE_URL must use scheme postgres, postgresql, sqlite, or file")
	}
}

// CanonicalSQLiteDSN builds the CLI-canonical sqlite:<abs-path> form.
func CanonicalSQLiteDSN(absPath string) string {
	p := filepath.Clean(absPath)
	p = filepath.ToSlash(p)
	return "sqlite:" + p
}

func sqlitePathFromRaw(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	// Windows: sqlite:C:/foo or sqlite:/C:/foo
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

func sqliteDriverDSN(path string) string {
	// modernc.org/sqlite accepts file: paths; use absolute-ish file URL.
	p := filepath.ToSlash(path)
	return "file:" + p + "?_pragma=foreign_keys(1)"
}

// ProbeHost normalizes a bind host for HTTP health/readiness probes.
func ProbeHost(host string) string {
	h := strings.TrimSpace(host)
	switch h {
	case "", "0.0.0.0":
		return "127.0.0.1"
	case "::", "[::]":
		return "::1"
	default:
		return h
	}
}

// ProbeURL builds an HTTP URL for local health/readiness checks.
func ProbeURL(apiURL, host, port, path string) string {
	if apiURL = strings.TrimSpace(apiURL); apiURL != "" {
		return strings.TrimRight(apiURL, "/") + path
	}
	ph := ProbeHost(host)
	addr := net.JoinHostPort(ph, port)
	return "http://" + addr + path
}

// ResolveConfigDir returns the directory containing server.yaml/.env.
// Order: DISCLOUD_CONFIG_DIR env → executable directory → empty (caller may use cwd/dev fallback).
func ResolveConfigDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("DISCLOUD_CONFIG_DIR")); d != "" {
		return filepath.Clean(d), nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

func validateLogLevel(level string) error {
	switch level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("DISCLOUD_LOG_LEVEL must be debug, info, warn, or error (got %q)", level)
	}
}

func parseOrigin(name, raw string) (origin string, secure bool, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", name, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false, fmt.Errorf("%s must use http or https scheme", name)
	}
	if u.Host == "" {
		return "", false, fmt.Errorf("%s must include a host", name)
	}
	if u.Path != "" && u.Path != "/" {
		return "", false, fmt.Errorf("%s must not include a path", name)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", false, fmt.Errorf("%s must not include query or fragment", name)
	}
	return u.Scheme + "://" + u.Host, u.Scheme == "https", nil
}

func validatePort(port string) error {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("DISCLOUD_PORT must be an integer 1–65535")
	}
	return nil
}

func validateURLScheme(name, raw string, schemes ...string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	scheme := strings.ToLower(u.Scheme)
	for _, s := range schemes {
		if scheme == s {
			if u.Host == "" {
				return fmt.Errorf("%s must include a host", name)
			}
			return nil
		}
	}
	return fmt.Errorf("%s must use scheme %s", name, strings.Join(schemes, " or "))
}

func hasToken(s string) bool {
	for _, p := range strings.Split(s, ",") {
		if strings.TrimSpace(p) != "" {
			return true
		}
	}
	return false
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseInt(v string, fallback int) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", v)
	}
	if n < 0 {
		return 0, fmt.Errorf("must be >= 0")
	}
	return n, nil
}

func parseBytes(v string, fallback int64) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte count %q", v)
	}
	if n < 0 {
		return 0, fmt.Errorf("must be >= 0")
	}
	return n, nil
}

// LoadOrCreateVisitorHashSalt returns the salt from path, or generates one and
// writes it atomically (temp file + rename) when missing/empty. If path is
// missing but the legacy .visitor_hash_salt sibling exists, it is renamed once.
func LoadOrCreateVisitorHashSalt(path string) (string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		legacy := filepath.Join(filepath.Dir(path), visitorHashSaltFileLegacy)
		_ = os.Rename(legacy, path) // best-effort migration
	}
	return loadOrCreateSecret(path, secretBytes)
}

func loadOrCreateSecret(path string, nbytes int) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	raw := make([]byte, nbytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate %s: %w", path, err)
	}
	secret := hex.EncodeToString(raw)

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o700)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(secret+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("persist %s: %w", path, err)
	}
	return secret, nil
}
