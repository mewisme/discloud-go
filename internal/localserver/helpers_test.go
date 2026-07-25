package localserver

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mewisme/discloud-go/internal/serverconfig"
)

const testRepo = "mewisme/discloud-go"

// fakeGitHub serves release redirects, assets, and checksums for one repo.
type fakeGitHub struct {
	t         *testing.T
	latest    string
	assets    map[string][]byte // path → body
	hits      map[string]int
	rateLimit bool
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	return &fakeGitHub{t: t, assets: map[string][]byte{}, hits: map[string]int{}}
}

// addRelease registers an archive whose single member is the server binary.
func (g *fakeGitHub) addRelease(version, binaryContent string) {
	asset := (Source{}).AssetName(version)
	body := buildArchive(g.t, asset, binaryContent)
	base := "/" + testRepo + "/releases/download/" + version + "/"
	g.assets[base+asset] = body
	g.assets[base+checksumsName] = []byte(fmt.Sprintf("%s  %s\n%s  other.txt\n", sha256Hex(body), asset, sha256Hex(nil)))
	g.latest = version
}

func (g *fakeGitHub) start() *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.hits[r.URL.Path]++
		if g.rateLimit {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1700000000")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch {
		case r.URL.Path == "/"+testRepo+"/releases/latest":
			http.Redirect(w, r, "/"+testRepo+"/releases/tag/"+g.latest, http.StatusFound)
		case r.URL.Path == "/api/repos/"+testRepo+"/releases/latest":
			fmt.Fprintf(w, `{"tag_name":%q}`, g.latest)
		default:
			body, ok := g.assets[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(body)
		}
	}))
	g.t.Cleanup(srv.Close)
	return srv
}

func (g *fakeGitHub) source(srv *httptest.Server) Source {
	return Source{
		HTTP:         srv.Client(),
		Repo:         testRepo,
		ReleasesBase: srv.URL,
		APIBase:      srv.URL + "/api",
	}
}

// buildArchive produces a zip or tar.gz (matching the asset name) holding the
// release binary under its shipped name "discloud".
func buildArchive(t *testing.T, asset, content string) []byte {
	t.Helper()
	name := "discloud"
	if runtime.GOOS == "windows" {
		name = "discloud.exe"
	}
	if strings.HasSuffix(asset, ".zip") {
		return zipBytes(t, map[string]string{name: content, "README.md": "docs"})
	}
	return tarGzBytes(t, map[string]string{name: content, "README.md": "docs"})
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
		hdr.SetMode(0o755)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tarGzBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     path.Clean(name),
			Mode:     0o755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func testPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	return Paths{InstallDir: filepath.Join(root, "server"), DataDir: filepath.Join(root, "data")}
}

// saveProbeConfig points the local probe at a stub HTTP server.
func saveProbeConfig(p Paths, apiURL string) error {
	return serverconfig.Save(p.InstallDir, serverconfig.File{Host: "127.0.0.1", Port: "6373", APIURL: apiURL})
}
