package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotEnvFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := `
# comment
DISCLOUD_BASE=http://api.test:8080
DISCLOUD_ORIGIN="http://app.test:3000"
export API_URL=http://ignored-when-discloud-set
WEB_ORIGIN='http://ignored-too'
EMPTY=
NOEQ
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := parseDotEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if m["DISCLOUD_BASE"] != "http://api.test:8080" {
		t.Fatalf("base: %q", m["DISCLOUD_BASE"])
	}
	if m["DISCLOUD_ORIGIN"] != "http://app.test:3000" {
		t.Fatalf("origin: %q", m["DISCLOUD_ORIGIN"])
	}
	if m["API_URL"] != "http://ignored-when-discloud-set" {
		t.Fatalf("api: %q", m["API_URL"])
	}
}

func TestLoadDotEnvFallbacks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("API_URL=http://from-api\nWEB_ORIGIN=http://from-web\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	base, origin := loadDotEnvValues()
	if base != "http://from-api" || origin != "http://from-web" {
		t.Fatalf("got base=%q origin=%q", base, origin)
	}
}

func TestDefaultConfigPrefersEnvOverDotEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DISCLOUD_BASE=http://from-dotenv\nDISCLOUD_ORIGIN=http://origin-dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	t.Setenv("DISCLOUD_BASE", "http://from-env")
	t.Setenv("DISCLOUD_ORIGIN", "http://origin-env")
	cfg := DefaultConfig()
	if cfg.BaseURL != "http://from-env" {
		t.Fatalf("base: %q", cfg.BaseURL)
	}
	if cfg.Origin != "http://origin-env" {
		t.Fatalf("origin: %q", cfg.Origin)
	}
}

func TestDefaultConfigUsesDotEnvWhenEnvEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DISCLOUD_BASE=http://from-dotenv/\nDISCLOUD_ORIGIN=http://origin-dotenv/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	t.Setenv("DISCLOUD_BASE", "")
	t.Setenv("DISCLOUD_ORIGIN", "")
	cfg := DefaultConfig()
	if cfg.BaseURL != "http://from-dotenv" {
		t.Fatalf("base: %q", cfg.BaseURL)
	}
	if cfg.Origin != "http://origin-dotenv" {
		t.Fatalf("origin: %q", cfg.Origin)
	}
}
