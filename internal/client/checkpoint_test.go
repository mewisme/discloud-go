package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	size, mtime := fileFingerprint(st)
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}

	cp := &uploadCheckpoint{
		UploadID:    "upid",
		ResumeToken: "tok",
		Path:        abs,
		Size:        size,
		ModTimeUnix: mtime,
		ChunkSize:   4,
		FileName:    "blob.bin",
		Hashes:      map[int]string{0: "abc"},
	}
	if err := saveCheckpoint(cp); err != nil {
		t.Fatal(err)
	}
	got, err := loadCheckpoint(path, size, mtime)
	if err != nil {
		t.Fatal(err)
	}
	if got.UploadID != "upid" || got.ResumeToken != "tok" || got.Hashes[0] != "abc" {
		t.Fatalf("got %+v", got)
	}

	if _, err := loadCheckpoint(path, size+1, mtime); err == nil {
		t.Fatal("want mismatch on size")
	}
	clearCheckpoint(path, size, mtime)
	if _, err := loadCheckpoint(path, size, mtime); err == nil {
		t.Fatal("want miss after clear")
	}
}
