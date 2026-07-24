package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcceptUploadPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, done, err := acceptUploadPath(nil, "  ")
	if err != nil || !done || out != nil {
		t.Fatalf("empty: out=%v done=%v err=%v", out, done, err)
	}

	out, done, err = acceptUploadPath(nil, file)
	if err != nil || done || len(out) != 1 || out[0] != file {
		t.Fatalf("file: out=%v done=%v err=%v", out, done, err)
	}

	out, done, err = acceptUploadPath(out, dir)
	if err != nil || done || len(out) != 2 || out[1] != dir {
		t.Fatalf("dir: out=%v done=%v err=%v", out, done, err)
	}

	_, done, err = acceptUploadPath(nil, filepath.Join(dir, "missing.bin"))
	if err == nil || done {
		t.Fatal("expected missing path error")
	}

	out, done, err = acceptUploadPath(out, "")
	if err != nil || !done || len(out) != 2 {
		t.Fatalf("finish: out=%v done=%v err=%v", out, done, err)
	}
}
