package serverconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	serverRoot := filepath.Join(root, "server")
	f := DefaultLocal("sqlite:/tmp/x.db")
	f.BotTokens = []string{"token-one-abcdef"}
	f.ChannelID = "123"
	if err := Save(serverRoot, f); err != nil {
		t.Fatal(err)
	}
	got, err := Load(serverRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got.ChannelID != "123" || got.Host != "127.0.0.1" {
		t.Fatalf("%+v", got)
	}
	// current presentation exists
	cur := filepath.Join(CurrentDir(serverRoot), YAMLName)
	if _, err := os.Stat(cur); err != nil {
		t.Fatal(err)
	}
}

func TestMaskSecrets(t *testing.T) {
	f := File{BotTokens: []string{"abcdefghijklmnop"}, DatabaseURL: "postgres://user:pass@host/db"}
	m := MaskSecrets(f)
	if m.BotTokens[0] == f.BotTokens[0] {
		t.Fatal("token not masked")
	}
	if !contains(m.DatabaseURL, "***@") {
		t.Fatalf("url=%q", m.DatabaseURL)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}

func TestParseDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\n\nDISCLOUD_PORT=8080\nDISCLOUD_BOT_TOKEN=\"a b\"\nexport DISCLOUD_HOST=127.0.0.1\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := ParseDotEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if m["DISCLOUD_PORT"] != "8080" || m["DISCLOUD_BOT_TOKEN"] != "a b" || m["DISCLOUD_HOST"] != "127.0.0.1" {
		t.Fatalf("%v", m)
	}
}

func TestParseDotEnvBadLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("NOTVALID\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseDotEnvFile(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnvMap(t *testing.T) {
	f := File{Host: "127.0.0.1", Port: "6373", BotTokens: []string{"a", "b"}, ChannelID: "1", CORS: "http://x"}
	m := EnvMap(f)
	if m["DISCLOUD_BOT_TOKEN"] != "a,b" {
		t.Fatalf("%v", m)
	}
}

func TestUnset(t *testing.T) {
	root := filepath.Join(t.TempDir(), "server")
	f := DefaultLocal("sqlite:/tmp/x.db")
	f.ChannelID = "99"
	if err := Save(root, f); err != nil {
		t.Fatal(err)
	}
	if err := Unset(root, "channelId"); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(root)
	if got.ChannelID != "" {
		t.Fatalf("%+v", got)
	}
}
