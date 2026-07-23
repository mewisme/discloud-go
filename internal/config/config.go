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
	// WebOrigin is the exact browser origin allowed for CORS credentials (e.g. http://localhost:3000).
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

func Load() (Config, error) {
	c := Config{
		Port:                    getenv("PORT", "8080"),
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		ValkeyURL:               os.Getenv("VALKEY_URL"),
		DiscordBotToken:         strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		DiscordChannelID:        os.Getenv("DISCORD_CHANNEL_ID"),
		APIURL:                  strings.TrimSpace(os.Getenv("API_URL")),
		AppSecret:               os.Getenv("APP_SECRET"),
		WebOrigin:               strings.TrimSpace(os.Getenv("WEB_ORIGIN")),
		TrustProxy:              envTruthy("TRUST_PROXY"),
		RateLimitUploadPerMin:   envInt("RATE_LIMIT_UPLOAD_PER_MIN", 60),
		RateLimitDownloadPerMin: envInt("RATE_LIMIT_DOWNLOAD_PER_MIN", 120),
		MaxUserBytes:            envBytes("MAX_USER_BYTES", 0),
		MaxAnonUploadsPerDay:    envInt("MAX_ANON_UPLOADS_PER_DAY", 50),
		MaxAnonBytesPerDay:      envBytes("MAX_ANON_BYTES_PER_DAY", 2<<30),
		MaxRawUploadBytes:       envBytes("MAX_RAW_UPLOAD_BYTES", 0),
		CaptchaSecret:           strings.TrimSpace(os.Getenv("CAPTCHA_SECRET")),
	}
	for name, v := range map[string]string{
		"DATABASE_URL":       c.DatabaseURL,
		"VALKEY_URL":         c.ValkeyURL,
		"DISCORD_BOT_TOKEN":  c.DiscordBotToken,
		"DISCORD_CHANNEL_ID": c.DiscordChannelID,
		"WEB_ORIGIN":         c.WebOrigin,
	} {
		if v == "" {
			return Config{}, fmt.Errorf("missing required environment variable %s", name)
		}
	}
	if c.AppSecret == "" {
		s, err := loadOrCreateSecret(AppSecretFile, secretBytes)
		if err != nil {
			return Config{}, err
		}
		c.AppSecret = s
	}
	if len(c.AppSecret) < 32 {
		return Config{}, fmt.Errorf("APP_SECRET must be at least 32 characters")
	}
	if !hasToken(c.DiscordBotToken) {
		return Config{}, fmt.Errorf("DISCORD_BOT_TOKEN has no usable tokens")
	}
	origin, secure, err := parseWebOrigin(c.WebOrigin)
	if err != nil {
		return Config{}, err
	}
	c.WebOrigin = origin
	c.CookieSecure = secure
	return c, nil
}

func parseWebOrigin(raw string) (origin string, secure bool, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false, fmt.Errorf("WEB_ORIGIN: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false, fmt.Errorf("WEB_ORIGIN must use http or https scheme")
	}
	if u.Host == "" {
		return "", false, fmt.Errorf("WEB_ORIGIN must include a host")
	}
	if u.Path != "" && u.Path != "/" {
		return "", false, fmt.Errorf("WEB_ORIGIN must not include a path")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", false, fmt.Errorf("WEB_ORIGIN must not include query or fragment")
	}
	return u.Scheme + "://" + u.Host, u.Scheme == "https", nil
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

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// envBytes parses a plain integer byte count (no units). Empty → fallback.
func envBytes(key string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
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
