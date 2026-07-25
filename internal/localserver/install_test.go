package localserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mewisme/discloud-go/internal/config"
	"github.com/mewisme/discloud-go/internal/serverconfig"
)

func TestInstallCreatesConfigActivatesAndDoesNotStart(t *testing.T) {
	g := newFakeGitHub(t)
	g.addRelease("v1.0.0", "v1-binary")
	srv := g.start()
	p := testPaths(t)
	o := Options{Paths: p, Source: g.source(srv)}

	res, err := Install(context.Background(), o, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != "v1.0.0" || !res.ConfigCreated {
		t.Fatalf("unexpected result %+v", res)
	}
	if data, err := os.ReadFile(p.CurrentBinary()); err != nil || string(data) != "v1-binary" {
		t.Fatalf("current binary content=%q err=%v", data, err)
	}
	if got := ActiveVersion(p); got != "v1.0.0" {
		t.Fatalf("ActiveVersion=%q", got)
	}
	if _, err := os.Stat(p.PIDPath()); !os.IsNotExist(err) {
		t.Fatal("install must not start the server")
	}

	f, err := serverconfig.Load(p.InstallDir)
	if err != nil {
		t.Fatal(err)
	}
	if want := config.CanonicalSQLiteDSN(p.DatabasePath()); f.DatabaseURL != want {
		t.Fatalf("DatabaseURL=%q want %q", f.DatabaseURL, want)
	}
	// current/ presents the config beside the binary.
	if _, err := os.Stat(filepath.Join(p.CurrentDir(), serverconfig.YAMLName)); err != nil {
		t.Fatalf("config not synced into current/: %v", err)
	}
}

func TestInstallTwiceKeepsConfigAndEnv(t *testing.T) {
	g := newFakeGitHub(t)
	g.addRelease("v1.0.0", "v1-binary")
	srv := g.start()
	p := testPaths(t)
	o := Options{Paths: p, Source: g.source(srv)}
	ctx := context.Background()

	if _, err := Install(ctx, o, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	// User customises config and adds secrets in .env.
	f, _ := serverconfig.Load(p.InstallDir)
	f.Port = "7777"
	if err := serverconfig.Save(p.InstallDir, f); err != nil {
		t.Fatal(err)
	}
	envPath := serverconfig.EnvPath(p.InstallDir)
	if err := os.WriteFile(envPath, []byte("DISCLOUD_BOT_TOKEN=keepme\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	g.addRelease("v2.0.0", "v2-binary")
	res, err := Install(ctx, o, "v2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if res.ConfigCreated {
		t.Fatal("existing config must not be recreated")
	}
	after, _ := serverconfig.Load(p.InstallDir)
	if after.Port != "7777" {
		t.Fatalf("config overwritten: port=%q", after.Port)
	}
	if data, _ := os.ReadFile(envPath); string(data) != "DISCLOUD_BOT_TOKEN=keepme\n" {
		t.Fatalf(".env overwritten: %q", data)
	}
	if data, err := os.ReadFile(p.CurrentBinary()); err != nil || string(data) != "v2-binary" {
		t.Fatalf("current binary not updated: %q %v", data, err)
	}
	// Both versions stay on disk.
	for _, v := range []string{"v1.0.0", "v2.0.0"} {
		if _, err := os.Stat(p.VersionBinary(v)); err != nil {
			t.Fatalf("missing %s: %v", v, err)
		}
	}
}

func TestUpdateKeepsDataAndReportsPrevious(t *testing.T) {
	g := newFakeGitHub(t)
	g.addRelease("v1.0.0", "v1-binary")
	srv := g.start()
	p := testPaths(t)
	o := Options{Paths: p, Source: g.source(srv)}
	ctx := context.Background()

	if _, err := Install(ctx, o, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.DatabasePath(), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}

	g.addRelease("v2.0.0", "v2-binary")
	res, err := Update(ctx, o, "v2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if res.PreviousVersion != "v1.0.0" || res.Version != "v2.0.0" {
		t.Fatalf("unexpected result %+v", res)
	}
	if res.Restarted {
		t.Fatal("server was not running; it must not be started")
	}
	if data, _ := os.ReadFile(p.DatabasePath()); string(data) != "db" {
		t.Fatalf("database touched: %q", data)
	}
	if got := ActiveVersion(p); got != "v2.0.0" {
		t.Fatalf("ActiveVersion=%q", got)
	}
}

func TestUpdateFailureKeepsPreviousActive(t *testing.T) {
	g := newFakeGitHub(t)
	g.addRelease("v1.0.0", "v1-binary")
	srv := g.start()
	p := testPaths(t)
	o := Options{Paths: p, Source: g.source(srv)}
	ctx := context.Background()

	if _, err := Install(ctx, o, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(ctx, o, "v9.9.9"); err == nil {
		t.Fatal("expected download failure for missing release")
	}
	if got := ActiveVersion(p); got != "v1.0.0" {
		t.Fatalf("ActiveVersion=%q after failed update", got)
	}
	if data, err := os.ReadFile(p.CurrentBinary()); err != nil || string(data) != "v1-binary" {
		t.Fatalf("current binary lost: %q %v", data, err)
	}
}

func TestUninstallKeepsDataUnlessPurged(t *testing.T) {
	g := newFakeGitHub(t)
	g.addRelease("v1.0.0", "v1-binary")
	srv := g.start()
	p := testPaths(t)
	o := Options{Paths: p, Source: g.source(srv)}
	ctx := context.Background()

	if _, err := Install(ctx, o, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.DatabasePath(), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.LogDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.LogPath(), []byte("log\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(p.DataDir, config.VisitorHashSaltFile)
	if err := os.WriteFile(secret, []byte("salt"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(ctx, o, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.VersionsDir()); !os.IsNotExist(err) {
		t.Fatal("versions/ should be gone")
	}
	if _, err := os.Lstat(p.CurrentBinary()); !os.IsNotExist(err) {
		t.Fatal("current binary should be gone")
	}
	for _, keep := range []string{p.ConfigPath(), p.DatabasePath(), p.LogPath(), secret} {
		if _, err := os.Stat(keep); err != nil {
			t.Fatalf("uninstall removed %s: %v", keep, err)
		}
	}

	if err := Uninstall(ctx, o, true); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{p.InstallDir, p.ConfigPath(), p.DatabasePath(), p.LogPath(), secret} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("purge kept %s", gone)
		}
	}
}
