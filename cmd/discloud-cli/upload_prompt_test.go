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
	file2 := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, empty, err := acceptUploadPath(nil, "  ")
	if err != nil || !empty || out != nil {
		t.Fatalf("empty: out=%v empty=%v err=%v", out, empty, err)
	}

	out, empty, err = acceptUploadPath(nil, file)
	if err != nil || empty || len(out) != 1 || out[0] != file {
		t.Fatalf("file: out=%v empty=%v err=%v", out, empty, err)
	}

	out, empty, err = acceptUploadPath(out, dir)
	if err != nil || empty || len(out) != 2 || out[1] != dir {
		t.Fatalf("dir: out=%v empty=%v err=%v", out, empty, err)
	}

	_, empty, err = acceptUploadPath(nil, filepath.Join(dir, "missing.bin"))
	if err == nil || empty {
		t.Fatal("expected missing path error")
	}

	csv := file + "," + file2
	out, empty, err = acceptUploadPath(nil, csv)
	if err != nil || empty || len(out) != 2 {
		t.Fatalf("csv: out=%v empty=%v err=%v", out, empty, err)
	}

	out, empty, err = acceptUploadPath(out, "")
	if err != nil || !empty || len(out) != 2 {
		t.Fatalf("finish empty: out=%v empty=%v err=%v", out, empty, err)
	}
}

func TestSplitCommaFields(t *testing.T) {
	t.Parallel()
	if got := splitCommaFields("a, b , ,c"); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("%v", got)
	}
	if got := splitCommaFields("  solo  "); len(got) != 1 || got[0] != "solo" {
		t.Fatalf("%v", got)
	}
	if got := splitCommaFields("  "); got != nil {
		t.Fatalf("%v", got)
	}
}
