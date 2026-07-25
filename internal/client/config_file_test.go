package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSaveConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	path, base, origin, err := SaveConfigFile("http://api.example/", "http://app.example/")
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(dir, "discloud", "config.yaml")
	if path != wantPath {
		t.Fatalf("path=%q want %q", path, wantPath)
	}
	if base != "http://api.example" || origin != "http://app.example" {
		t.Fatalf("saved %q %q", base, origin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f FileConfig
	if err := yaml.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if f.Base != "http://api.example" || f.Origin != "http://app.example" {
		t.Fatalf("%+v", f)
	}

	_, base2, origin2, err := SaveConfigFile("http://api2.example", "")
	if err != nil {
		t.Fatal(err)
	}
	if base2 != "http://api2.example" || origin2 != "http://app.example" {
		t.Fatalf("merge got %q %q", base2, origin2)
	}
}

func TestMigrateJSONToYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "discloud")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := FileConfig{Base: "http://api.legacy", Origin: "http://app.legacy", Token: "dc_secret"}
	raw, _ := json.MarshalIndent(legacy, "", "  ")
	jsonPath := filepath.Join(cfgDir, "config.json")
	if err := os.WriteFile(jsonPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureConfigMigrated(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfgDir, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Fatalf("json should be archived, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(cfgDir, "config.json.migrated")); err != nil {
		t.Fatal(err)
	}
	f, err := LoadFileConfig()
	if err != nil {
		t.Fatal(err)
	}
	if f.Base != "http://api.legacy" || f.Token != "dc_secret" {
		t.Fatalf("%+v", f)
	}
	// idempotent
	if err := EnsureConfigMigrated(); err != nil {
		t.Fatal(err)
	}
}

func TestSelfUploadSaveLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	su := SelfUploadConfig{
		BotTokens:   []string{" tok-a ", "tok-b", "tok-a", ""},
		ChannelID:   "111",
		GuildID:     "222",
		PremiumTier: 2,
		ChunkSize:   50 << 20,
	}
	if err := SaveSelfUpload(&su); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSelfUpload()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.BotTokens) != 2 || got.BotTokens[0] != "tok-a" || got.ChannelID != "111" {
		t.Fatalf("%+v", got)
	}
	if !got.Configured() {
		t.Fatal("expected configured")
	}
}

func TestNormalizeBotTokens(t *testing.T) {
	got := NormalizeBotTokens([]string{" b ", "", "a", "b"})
	if len(got) != 2 || got[0] != "b" || got[1] != "a" {
		t.Fatalf("%v", got)
	}
}
