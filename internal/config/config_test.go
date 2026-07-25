package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
		"DISCLOUD_DATABASE_URL", "DISCLOUD_BOT_TOKEN",
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

func TestMergePrecedence(t *testing.T) {
	merged := Merge(Sources{
		Defaults: map[string]string{"DISCLOUD_PORT": "1", "A": "defaults"},
		YAML:     map[string]string{"DISCLOUD_PORT": "2", "A": "yaml", "B": "yaml"},
		DotEnv:   map[string]string{"DISCLOUD_PORT": "3", "B": "dotenv", "C": "dotenv"},
		Process:  map[string]string{"DISCLOUD_PORT": "4", "C": "process"},
	})
	if merged["DISCLOUD_PORT"] != "4" {
		t.Fatalf("port=%q", merged["DISCLOUD_PORT"])
	}
	if merged["A"] != "yaml" || merged["B"] != "dotenv" || merged["C"] != "process" {
		t.Fatalf("merged=%v", merged)
	}
}

func TestMergeDoesNotMutateProcessEnv(t *testing.T) {
	t.Setenv("DISCLOUD_PORT", "9999")
	before := os.Getenv("DISCLOUD_PORT")
	_ = Merge(Sources{
		Defaults: DefaultsMap(),
		Process:  ProcessEnvMap(),
		YAML:     map[string]string{"DISCLOUD_PORT": "1111"},
	})
	if os.Getenv("DISCLOUD_PORT") != before {
		t.Fatalf("process env mutated")
	}
}

func TestSQLiteAllowsEmptyValkey(t *testing.T) {
	chdirTemp(t)
	db := filepath.Join(t.TempDir(), "discloud.db")
	t.Setenv("DISCLOUD_DATABASE_URL", CanonicalSQLiteDSN(db))
	os.Unsetenv("DISCLOUD_VALKEY_URL")
	t.Setenv("DISCLOUD_BOT_TOKEN", "tok")
	t.Setenv("DISCLOUD_CHANNEL_ID", "1")
	t.Setenv("DISCLOUD_CORS", "http://localhost:3000")
	t.Setenv("DISCLOUD_APP_SECRET", "0123456789abcdef0123456789abcdef")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend != BackendSQLite {
		t.Fatalf("backend=%s", cfg.Backend)
	}
	if cfg.CacheBackendName() != "memory" {
		t.Fatalf("cache=%s", cfg.CacheBackendName())
	}
}

func TestPostgresRequiresValkey(t *testing.T) {
	chdirTemp(t)
	t.Setenv("DISCLOUD_DATABASE_URL", "postgres://x")
	os.Unsetenv("DISCLOUD_VALKEY_URL")
	t.Setenv("DISCLOUD_BOT_TOKEN", "tok")
	t.Setenv("DISCLOUD_CHANNEL_ID", "1")
	t.Setenv("DISCLOUD_CORS", "http://localhost:3000")
	t.Setenv("DISCLOUD_APP_SECRET", "0123456789abcdef0123456789abcdef")
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseDatabaseURLSQLiteWindows(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"sqlite:C:/data/discloud.db", filepath.FromSlash("C:/data/discloud.db")},
		{"sqlite:/C:/data/discloud.db", filepath.FromSlash("C:/data/discloud.db")},
		{"file:C:/data/discloud.db", filepath.FromSlash("C:/data/discloud.db")},
		{"sqlite:/tmp/discloud.db", filepath.FromSlash("/tmp/discloud.db")},
	}
	for _, tc := range cases {
		info, err := ParseDatabaseURL(tc.raw)
		if err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		if info.Backend != BackendSQLite {
			t.Fatalf("%s backend=%s", tc.raw, info.Backend)
		}
		if runtime.GOOS == "windows" && strings.HasPrefix(tc.want, string(filepath.Separator)) && !strings.Contains(tc.want, ":") {
			// Unix-style path on Windows FromSlash keeps leading slash — compare cleaned.
		}
		if filepath.Clean(info.Path) != filepath.Clean(tc.want) {
			t.Fatalf("%s path=%q want %q", tc.raw, info.Path, tc.want)
		}
	}
}

func TestProbeHostAndURL(t *testing.T) {
	if ProbeHost("") != "127.0.0.1" || ProbeHost("0.0.0.0") != "127.0.0.1" {
		t.Fatal(ProbeHost(""), ProbeHost("0.0.0.0"))
	}
	if ProbeHost("::") != "::1" || ProbeHost("[::]") != "::1" {
		t.Fatal(ProbeHost("::"))
	}
	u := ProbeURL("", "0.0.0.0", "6373", "/readyz")
	if u != "http://127.0.0.1:6373/readyz" {
		t.Fatalf("u=%q", u)
	}
	u6 := ProbeURL("", "::", "6373", "/readyz")
	if !strings.Contains(u6, "[::1]:6373") {
		t.Fatalf("u6=%q", u6)
	}
	if ProbeURL("http://api.example.com", "0.0.0.0", "1", "/readyz") != "http://api.example.com/readyz" {
		t.Fatal("apiUrl preferred")
	}
}

func TestLogLevelValidation(t *testing.T) {
	chdirTemp(t)
	setRequiredEnv(t)
	t.Setenv("DISCLOUD_APP_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("DISCLOUD_LOG_LEVEL", "trace")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid log level")
	}
	t.Setenv("DISCLOUD_LOG_LEVEL", "debug")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("level=%q", cfg.LogLevel)
	}
}

func TestListenAddr(t *testing.T) {
	c := Config{Host: "127.0.0.1", Port: "6373"}
	if c.ListenAddr() != "127.0.0.1:6373" {
		t.Fatal(c.ListenAddr())
	}
	c.Host = ""
	if c.ListenAddr() != ":6373" {
		t.Fatal(c.ListenAddr())
	}
}
