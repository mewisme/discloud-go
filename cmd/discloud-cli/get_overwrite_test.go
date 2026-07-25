package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mewisme/discloud-go/internal/selfmanaged"
)

func TestExistingDownloadPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nested := filepath.Join(dir, "Folder", "child")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(nested, "a.txt")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	files := []selfmanaged.FileMeta{
		{LogicalPath: "Folder/child/a.txt", OriginalName: "a.txt"},
		{LogicalPath: "Folder/child/b.txt", OriginalName: "b.txt"},
	}
	got := existingDownloadPaths(dir, files)
	if len(got) != 1 || got[0] != existing {
		t.Fatalf("conflicts = %v, want [%s]", got, existing)
	}
}

func TestNextIndexedDest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(base, []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}
	one := filepath.Join(dir, "a.1.txt")
	if err := os.WriteFile(one, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := nextIndexedDest(base, nil)
	want := filepath.Join(dir, "a.2.txt")
	if got != want {
		t.Fatalf("nextIndexedDest = %q, want %q", got, want)
	}

	reserved := map[string]struct{}{want: {}}
	got = nextIndexedDest(base, reserved)
	want3 := filepath.Join(dir, "a.3.txt")
	if got != want3 {
		t.Fatalf("nextIndexedDest reserved = %q, want %q", got, want3)
	}
}

func TestIndexConflictingDownloads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nested := filepath.Join(dir, "Folder")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	files := []selfmanaged.FileMeta{
		{LogicalPath: "Folder/a.txt", OriginalName: "a.txt"},
		{LogicalPath: "Folder/b.txt", OriginalName: "b.txt"},
	}
	got, err := indexConflictingDownloads(dir, files)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].LogicalPath != "Folder/a.1.txt" {
		t.Fatalf("indexed path = %q, want Folder/a.1.txt", got[0].LogicalPath)
	}
	if got[1].LogicalPath != "Folder/b.txt" {
		t.Fatalf("free path changed: %q", got[1].LogicalPath)
	}
}
