package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandFolderJobs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	folder := filepath.Join(root, "MyFolder")
	if err := os.MkdirAll(filepath.Join(folder, "child", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite := func(rel, body string) {
		p := filepath.Join(folder, filepath.FromSlash(rel))
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("a.txt", "aaa")
	mustWrite("child/b.txt", "bb")
	mustWrite("child/nested/c.txt", "c")
	mustWrite("empty.txt", "") // skipped

	jobs, err := expandFolderJobs(folder, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs=%d want 3: %+v", len(jobs), jobs)
	}
	got := map[string]bool{}
	for _, j := range jobs {
		got[j.remoteName] = true
	}
	for _, want := range []string{"MyFolder/a.txt", "MyFolder/child/b.txt", "MyFolder/child/nested/c.txt"} {
		if !got[want] {
			t.Fatalf("missing %s in %v", want, got)
		}
	}
}

func TestExpandFolderJobsDepth(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	folder := filepath.Join(root, "MyFolder")
	if err := os.MkdirAll(filepath.Join(folder, "child", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite := func(rel, body string) {
		p := filepath.Join(folder, filepath.FromSlash(rel))
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("a.txt", "aaa")
	mustWrite("child/b.txt", "bb")
	mustWrite("child/nested/c.txt", "c")

	jobs, err := expandFolderJobs(folder, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].remoteName != "MyFolder/a.txt" {
		t.Fatalf("depth=1 got %+v want only MyFolder/a.txt", jobs)
	}

	jobs, err = expandFolderJobs(folder, 2)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, j := range jobs {
		got[j.remoteName] = true
	}
	if len(got) != 2 || !got["MyFolder/a.txt"] || !got["MyFolder/child/b.txt"] {
		t.Fatalf("depth=2 got %v", got)
	}
}
