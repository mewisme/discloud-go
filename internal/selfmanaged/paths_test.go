package selfmanaged

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDiscloudExt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    string
		changed bool
	}{
		{"", "", false},
		{"  file.discloud  ", "file.discloud", false},
		{"file.DISCLOUD", "file.DISCLOUD", false},
		{"file", "file.discloud", true},
		{"dir/file", "dir/file.discloud", true},
		{"file.disclou", "file.discloud", true},
		{"file.dis", "file.discloud", true},
		{"file.txt", "file.txt.discloud", true},
	}
	for _, tc := range cases {
		got, changed := EnsureDiscloudExt(tc.in)
		if got != tc.want || changed != tc.changed {
			t.Fatalf("%q: got (%q, %v) want (%q, %v)", tc.in, got, changed, tc.want, tc.changed)
		}
	}
}

func TestSafeJoinNestedLogicalPath(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	dest, err := SafeJoin(out, "MyFolder/child/nested/c.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(out, "MyFolder", "child", "nested", "c.txt")
	if dest != want {
		t.Fatalf("dest=%q want %q", dest, want)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(dest)
	if err != nil || st.IsDir() {
		t.Fatalf("expected file at nested path: %v", err)
	}
}
