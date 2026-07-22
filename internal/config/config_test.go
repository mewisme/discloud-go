package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("VALKEY_URL", "valkey://x")
	t.Setenv("DISCORD_BOT_TOKEN", "tok")
	t.Setenv("DISCORD_CHANNEL_ID", "1")
	t.Setenv("WEB_ORIGIN", "http://localhost:3000")
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
	os.Unsetenv("APP_SECRET")
	os.Unsetenv("WEB_ORIGIN")
	if _, err := Load(); err == nil {
		t.Fatal("expected error without WEB_ORIGIN")
	}
	t.Setenv("WEB_ORIGIN", "http://localhost:3000")
	t.Setenv("APP_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for short APP_SECRET")
	}
	t.Setenv("APP_SECRET", "0123456789abcdef0123456789abcdef")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WebOrigin != "http://localhost:3000" || cfg.CookieSecure {
		t.Fatalf("got %+v", cfg)
	}
	t.Setenv("WEB_ORIGIN", "https://app.example.com")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.CookieSecure {
		t.Fatal("https origin should set CookieSecure")
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
	t.Setenv("APP_SECRET", envSecret)
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
	os.Unsetenv("APP_SECRET")
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
