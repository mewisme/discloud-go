package localserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetName(t *testing.T) {
	s := Source{GOOS: "linux", GOARCH: "amd64"}
	if got := s.AssetName("v0.4.1"); got != "discloud_0.4.1_linux_amd64.tar.gz" {
		t.Fatalf("linux asset = %q", got)
	}
	w := Source{GOOS: "windows", GOARCH: "arm64"}
	if got := w.AssetName("v1.2.3"); got != "discloud_1.2.3_windows_arm64.zip" {
		t.Fatalf("windows asset = %q", got)
	}
}

func TestNormalizeVersionRejectsTraversal(t *testing.T) {
	if v, err := NormalizeVersion("0.4.1"); err != nil || v != "v0.4.1" {
		t.Fatalf("NormalizeVersion(0.4.1) = %q, %v", v, err)
	}
	for _, bad := range []string{"", "../../etc", "v1/../../x", `v1\..\x`} {
		if _, err := NormalizeVersion(bad); err == nil {
			t.Fatalf("expected rejection of %q", bad)
		}
	}
}

func TestResolveVersionExplicitSkipsNetwork(t *testing.T) {
	s := Source{HTTP: &http.Client{Transport: failTransport{t}}}
	got, err := s.ResolveVersion(context.Background(), "1.0.0")
	if err != nil || got != "v1.0.0" {
		t.Fatalf("got %q, %v", got, err)
	}
}

type failTransport struct{ t *testing.T }

func (f failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.t.Fatal("unexpected network call")
	return nil, nil
}

func TestResolveLatestByRedirect(t *testing.T) {
	g := newFakeGitHub(t)
	g.addRelease("v0.9.0", "binary")
	srv := g.start()

	got, err := g.source(srv).ResolveVersion(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "v0.9.0" {
		t.Fatalf("got %q", got)
	}
	if g.hits["/api/repos/"+testRepo+"/releases/latest"] != 0 {
		t.Fatal("API fallback should not run when the redirect works")
	}
}

func TestResolveLatestFallsBackToAPI(t *testing.T) {
	g := newFakeGitHub(t)
	g.addRelease("v0.9.0", "binary")
	srv := g.start()
	s := g.source(srv)
	s.ReleasesBase = srv.URL + "/missing" // redirect route 404s

	got, err := s.ResolveVersion(context.Background(), "latest")
	if err != nil {
		t.Fatal(err)
	}
	if got != "v0.9.0" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveLatestRateLimited(t *testing.T) {
	g := newFakeGitHub(t)
	g.addRelease("v0.9.0", "binary")
	g.rateLimit = true
	srv := g.start()

	_, err := g.source(srv).ResolveVersion(context.Background(), "")
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !strings.Contains(err.Error(), "--version") || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("error is not actionable: %v", err)
	}
}

func TestDownloadVerifiesChecksum(t *testing.T) {
	g := newFakeGitHub(t)
	g.addRelease("v0.9.0", "binary")
	srv := g.start()

	dir := t.TempDir()
	path, err := g.source(srv).Download(context.Background(), "v0.9.0", dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("downloaded outside dest: %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadRejectsBadChecksum(t *testing.T) {
	g := newFakeGitHub(t)
	g.addRelease("v0.9.0", "binary")
	srv := g.start()
	// Corrupt the served archive so its hash no longer matches checksums.txt.
	asset := "/" + testRepo + "/releases/download/v0.9.0/" + (Source{}).AssetName("v0.9.0")
	g.assets[asset] = append(g.assets[asset], 0xff)

	dir := t.TempDir()
	_, err := g.source(srv).Download(context.Background(), "v0.9.0", dir)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("bad download was kept: %v", entries)
	}
}

func TestFindChecksum(t *testing.T) {
	list := "aaa  short.txt\n" + strings.Repeat("b", 64) + "  discloud_1.0.0_linux_amd64.tar.gz\n"
	got, err := findChecksum(list, "discloud_1.0.0_linux_amd64.tar.gz")
	if err != nil || got != strings.Repeat("b", 64) {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := findChecksum(list, "missing.zip"); err == nil {
		t.Fatal("expected missing asset error")
	}
}

func TestTokenNeverAppearsInErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	s := Source{HTTP: srv.Client(), Repo: testRepo, ReleasesBase: srv.URL, APIBase: srv.URL, Token: "super-secret-token"}
	_, err := s.Download(context.Background(), "v1.0.0", t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("token leaked: %v", err)
	}
}
