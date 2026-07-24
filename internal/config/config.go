// Package config loads server configuration from environment variables.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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

type Config struct {
	Port        string
	DatabaseURL string
	ValkeyURL   string
	// DiscordBotToken is the raw env value: one token, or comma-separated
	// tokens that divide uploads across bots.
	DiscordBotToken  string
	DiscordChannelID string
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

	// Abuse protection (Phase 4). Zero rate limits disable that limiter.
	RateLimitUploadPerMin   int   // POST chunks / upload / session create+complete (per IP + per user)
	RateLimitDownloadPerMin int   // GET /f/{id} per IP
	MaxUserBytes            int64 // 0 = unlimited owned storage
	MaxAnonUploadsPerDay    int   // 0 = unlimited anon session creates per IP/day
	MaxAnonBytesPerDay      int64 // 0 = unlimited anon completed bytes per IP/day
	MaxRawUploadBytes       int64 // hard cap on POST /api/upload body (0 = max file size)
	// CaptchaSecret enables Turnstile siteverify on anon upload session create when set.
	CaptchaSecret string
}

// Load reads and validates environment variables. The API must not listen until
// this returns without error.
func Load() (Config, error) {
	c := Config{
		Port:             getenv("DISCLOUD_PORT", "8080"),
		DatabaseURL:      strings.TrimSpace(os.Getenv("DISCLOUD_DATABASE_URL")),
		ValkeyURL:        strings.TrimSpace(os.Getenv("DISCLOUD_VALKEY_URL")),
		DiscordBotToken:  strings.TrimSpace(os.Getenv("DISCLOUD_BOT_TOKEN")),
		DiscordChannelID: strings.TrimSpace(os.Getenv("DISCLOUD_CHANNEL_ID")),
		APIURL:           strings.TrimSpace(os.Getenv("DISCLOUD_API_URL")),
		AppSecret:        os.Getenv("DISCLOUD_APP_SECRET"),
		WebOrigin:        strings.TrimSpace(os.Getenv("DISCLOUD_CORS")),
		TrustProxy:       envTruthy("DISCLOUD_TRUST_PROXY"),
		CaptchaSecret:    strings.TrimSpace(os.Getenv("DISCLOUD_CAPTCHA_SECRET")),
	}

	var missing []string
	for _, kv := range []struct{ name, v string }{
		{"DISCLOUD_DATABASE_URL", c.DatabaseURL},
		{"DISCLOUD_VALKEY_URL", c.ValkeyURL},
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

	var err error
	if c.RateLimitUploadPerMin, err = envInt("DISCLOUD_RL_UPLOAD", 60); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if c.RateLimitDownloadPerMin, err = envInt("DISCLOUD_RL_DOWNLOAD", 120); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if c.MaxUserBytes, err = envBytes("DISCLOUD_MAX_USER_BYTES", 0); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if c.MaxAnonUploadsPerDay, err = envInt("DISCLOUD_MAX_ANON_UPLOADS", 50); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if c.MaxAnonBytesPerDay, err = envBytes("DISCLOUD_MAX_ANON_BYTES", 2<<30); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if c.MaxRawUploadBytes, err = envBytes("DISCLOUD_MAX_RAW_UPLOAD", 0); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	if err := validatePort(c.Port); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := validateURLScheme("DISCLOUD_DATABASE_URL", c.DatabaseURL, "postgres", "postgresql"); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := validateURLScheme("DISCLOUD_VALKEY_URL", c.ValkeyURL, "valkey", "redis"); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if c.APIURL != "" {
		origin, _, err := parseOrigin("DISCLOUD_API_URL", c.APIURL)
		if err != nil {
			return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		c.APIURL = origin
	}

	if c.AppSecret == "" {
		s, err := loadOrCreateSecret(AppSecretFile, secretBytes)
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

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envTruthy(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envInt(key string, fallback int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: must be an integer", key, v)
	}
	if n < 0 {
		return 0, fmt.Errorf("%s must be >= 0", key)
	}
	return n, nil
}

// envBytes parses a plain integer byte count (no units). Empty → fallback.
func envBytes(key string, fallback int64) (int64, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: must be an integer byte count", key, v)
	}
	if n < 0 {
		return 0, fmt.Errorf("%s must be >= 0", key)
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

// loadOrCreateSecret returns the trimmed secret from path, or generates nbytes
// of random data (hex-encoded) and writes it atomically when missing/empty.
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
