package localserver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractAndRenameBinary(t *testing.T) {
	for _, ext := range []string{".zip", ".tar.gz"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "release"+ext)
			var body []byte
			files := map[string]string{"discloud": "server-bytes", "LICENSE": "mit"}
			if ext == ".zip" {
				body = zipBytes(t, files)
			} else {
				body = tarGzBytes(t, files)
			}
			if err := os.WriteFile(archive, body, 0o600); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(dir, "out")
			if err := Extract(archive, dest); err != nil {
				t.Fatal(err)
			}
			bin, err := RenameServerBinary(dest)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(bin) != BinaryName() {
				t.Fatalf("binary not renamed: %s", bin)
			}
			data, err := os.ReadFile(bin)
			if err != nil || string(data) != "server-bytes" {
				t.Fatalf("content=%q err=%v", data, err)
			}
			if _, err := os.Stat(filepath.Join(dest, "LICENSE")); err != nil {
				t.Fatalf("sibling file missing: %v", err)
			}
		})
	}
}

func TestExtractRejectsZipTraversal(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "evil.zip")
	if err := os.WriteFile(archive, zipBytes(t, map[string]string{"../escaped.txt": "pwned"}), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Extract(archive, filepath.Join(dir, "out"))
	if err == nil || !strings.Contains(err.Error(), "unsafe archive entry") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped.txt")); err == nil {
		t.Fatal("file escaped the destination")
	}
}

func TestExtractRejectsTarTraversalAndAbsolute(t *testing.T) {
	for _, name := range []string{"../escaped.txt", "/tmp/absolute.txt", "sub/../../escaped.txt"} {
		dir := t.TempDir()
		archive := filepath.Join(dir, "evil.tar.gz")
		if err := os.WriteFile(archive, rawTarGz(t, name, "pwned"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := Extract(archive, filepath.Join(dir, "out"))
		if err == nil || !strings.Contains(err.Error(), "unsafe archive entry") {
			t.Fatalf("%s: expected rejection, got %v", name, err)
		}
	}
}

// rawTarGz writes a header name verbatim (no path.Clean) so traversal survives.
func rawTarGz(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractUnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "release.rar")
	if err := os.WriteFile(archive, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Extract(archive, filepath.Join(dir, "out")); err == nil {
		t.Fatal("expected unsupported archive error")
	}
}

func TestRenameServerBinaryMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RenameServerBinary(dir); err == nil {
		t.Fatal("expected missing binary error")
	}
}
