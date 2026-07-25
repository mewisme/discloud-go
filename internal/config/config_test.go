package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DISCLOUD_DATABASE_URL", "postgres://x")
	t.Setenv("DISCLOUD_VALKEY_URL", "valkey://x")
	t.Setenv("DISCLOUD_BOT_TOKEN", "tok")
	t.Setenv("DISCLOUD_CHANNEL_ID", "1")
	t.Setenv("DISCLOUD_CORS", "http://localhost:3000")
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	return dir
}

func TestLoadRequiresWebOrigin(t *testing.T) {
	chdirTemp(t)
	setRequiredEnv(t)
	os.Unsetenv("DISCLOUD_APP_SECRET")
	os.Unsetenv("DISCLOUD_CORS")
	if _, err := Load(); err == nil {
		t.Fatal("expected error without DISCLOUD_CORS")
	}
	t.Setenv("DISCLOUD_CORS", "http://localhost:3000")
	t.Setenv("DISCLOUD_APP_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for short DISCLOUD_APP_SECRET")
	}
	t.Setenv("DISCLOUD_APP_SECRET", "0123456789abcdef0123456789abcdef")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WebOrigin != "http://localhost:3000" || cfg.CookieSecure {
		t.Fatalf("got %+v", cfg)
	}
	t.Setenv("DISCLOUD_CORS", "https://app.example.com")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.CookieSecure {
		t.Fatal("https origin should set CookieSecure")
	}
}

func TestLoadRejectsMissingRequired(t *testing.T) {
	chdirTemp(t)
	for _, k := range []string{
		"DISCLOUD_DATABASE_URL", "DISCLOUD_VALKEY_URL", "DISCLOUD_BOT_TOKEN",
		"DISCLOUD_CHANNEL_ID", "DISCLOUD_CORS", "DISCLOUD_APP_SECRET",
	} {
		os.Unsetenv(k)
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing required env")
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"DISCLOUD_DATABASE_URL", "DISCLOUD_VALKEY_URL", "DISCLOUD_BOT_TOKEN",
		"DISCLOUD_CHANNEL_ID", "DISCLOUD_CORS",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %s", msg, want)
		}
	}
}

func TestLoadRejectsInvalidEnv(t *testing.T) {
	chdirTemp(t)
	setRequiredEnv(t)
	t.Setenv("DISCLOUD_APP_SECRET", "0123456789abcdef0123456789abcdef")

	t.Setenv("DISCLOUD_PORT", "notaport")
	if _, err := Load(); err == nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for port, got %v", err)
	}
	t.Setenv("DISCLOUD_PORT", "6373")

	t.Setenv("DISCLOUD_DATABASE_URL", "mysql://localhost/db")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid database URL error")
	}
	t.Setenv("DISCLOUD_DATABASE_URL", "postgres://x")

	t.Setenv("DISCLOUD_VALKEY_URL", "http://localhost:6379")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid valkey URL error")
	}
	t.Setenv("DISCLOUD_VALKEY_URL", "valkey://x")

	t.Setenv("DISCLOUD_RL_UPLOAD", "nope")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid RL_UPLOAD error")
	}
	t.Setenv("DISCLOUD_RL_UPLOAD", "-1")
	if _, err := Load(); err == nil {
		t.Fatal("expected negative RL_UPLOAD error")
	}
	os.Unsetenv("DISCLOUD_RL_UPLOAD")

	t.Setenv("DISCLOUD_API_URL", "http://api.example.com/path")
	if _, err := Load(); err == nil {
		t.Fatal("expected API_URL with path to fail")
	}
	t.Setenv("DISCLOUD_API_URL", "http://api.example.com")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "http://api.example.com" {
		t.Fatalf("APIURL=%q", cfg.APIURL)
	}
}

func TestLoadAppSecretEnvWinsOverFile(t *testing.T) {
	chdirTemp(t)
	setRequiredEnv(t)
	fileSecret := "file-secret-at-least-32-chars-long!"
	if err := os.WriteFile(AppSecretFile, []byte(fileSecret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envSecret := "env-secret-at-least-32-characters!!"
	t.Setenv("DISCLOUD_APP_SECRET", envSecret)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppSecret != envSecret {
		t.Fatalf("env should win: got %q want %q", cfg.AppSecret, envSecret)
	}
}

func TestLoadAppSecretCreatesFileWhenEnvMissing(t *testing.T) {
	chdirTemp(t)
	setRequiredEnv(t)
	os.Unsetenv("DISCLOUD_APP_SECRET")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AppSecret) < 32 {
		t.Fatalf("secret too short: %q", cfg.AppSecret)
	}
	raw, err := os.ReadFile(AppSecretFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != cfg.AppSecret {
		t.Fatalf("file %q != secret %q", raw, cfg.AppSecret)
	}
	// Stable across reloads.
	cfg2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.AppSecret != cfg.AppSecret {
		t.Fatalf("secret not stable: %q vs %q", cfg.AppSecret, cfg2.AppSecret)
	}
}

func TestLoadOrCreateVisitorHashSalt(t *testing.T) {
	path := filepath.Join(t.TempDir(), VisitorHashSaltFile)
	s1, err := LoadOrCreateVisitorHashSalt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s1) != secretBytes*2 {
		t.Fatalf("salt len=%d want %d", len(s1), secretBytes*2)
	}
	s2, err := LoadOrCreateVisitorHashSalt(path)
	if err != nil {
		t.Fatal(err)
	}
	if s1 != s2 {
		t.Fatalf("salt not stable: %q vs %q", s1, s2)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != s1 {
		t.Fatalf("file content %q != salt %q", raw, s1)
	}
}

func TestLoadOrCreateVisitorHashSaltMigratesLegacy(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, visitorHashSaltFileLegacy)
	path := filepath.Join(dir, VisitorHashSaltFile)
	const want = "legacy-visitor-salt-value"
	if err := os.WriteFile(legacy, []byte(want+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadOrCreateVisitorHashSalt(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("new path missing: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy should be gone after rename, err=%v", err)
	}
}
