package selfmanaged

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigIdentityOrderIndependent(t *testing.T) {
	a := ComputeConfigIdentity("g", "c", []string{"tok-a", "tok-b", "tok-c"})
	b := ComputeConfigIdentity("g", "c", []string{"tok-c", "tok-a", "tok-b"})
	if a != b {
		t.Fatalf("order changed hash: %s vs %s", a, b)
	}
	c := ComputeConfigIdentity("g", "c", []string{"tok-a", "tok-b", "tok-c", "tok-a", " tok-b "})
	if a != c {
		t.Fatalf("dupes/whitespace changed hash")
	}
	d := ComputeConfigIdentity("g", "c", []string{"tok-a", "tok-b", "tok-d"})
	if a == d {
		t.Fatal("different token should change hash")
	}
	e := ComputeConfigIdentity("g", "other", []string{"tok-a", "tok-b", "tok-c"})
	if a == e {
		t.Fatal("channel should change hash")
	}
	f := ComputeConfigIdentity("other", "c", []string{"tok-a", "tok-b", "tok-c"})
	if a == f {
		t.Fatal("guild should change hash")
	}
	if !IdentitiesMatch(a, b) {
		t.Fatal("IdentitiesMatch failed")
	}
}

func TestObfuscateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.discloud")
	meta := &BundleMeta{
		BundleID:       "b1",
		ConfigIdentity: "abc",
		GuildID:        "g",
		ChannelID:      "c",
		Files: []FileMeta{{
			ID: "f1", OriginalName: "a.txt", LogicalPath: "a.txt", Size: 3, SHA256: "x",
		}},
	}
	meta.RecalcTotals()
	if err := Encode(path, meta, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	if !DetectBundle(path) {
		t.Fatal("DetectBundle")
	}
	got, err := Decode(path, DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.BundleID != "b1" || got.Protection != ProtectionObfuscated {
		t.Fatalf("%+v", got)
	}
}

func TestPasswordRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.discloud")
	meta := &BundleMeta{BundleID: "enc", ConfigIdentity: "id"}
	if err := Encode(path, meta, EncodeOptions{Password: "secretpass"}); err != nil {
		t.Fatal(err)
	}
	mode, err := DecodePeekMode(path)
	if err != nil || mode != ProtectionPassword {
		t.Fatalf("mode=%s err=%v", mode, err)
	}
	_, err = Decode(path, DecodeOptions{})
	if err != ErrPasswordRequired {
		t.Fatalf("want password required, got %v", err)
	}
	_, err = Decode(path, DecodeOptions{Password: "wrongpassword"})
	if err != ErrCorruptBundle {
		t.Fatalf("want corrupt, got %v", err)
	}
	got, err := Decode(path, DecodeOptions{Password: "secretpass"})
	if err != nil {
		t.Fatal(err)
	}
	if got.BundleID != "enc" {
		t.Fatal(got.BundleID)
	}
}

func TestSafeJoin(t *testing.T) {
	dir := t.TempDir()
	p, err := SafeJoin(dir, "sub/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "file.txt" {
		t.Fatal(p)
	}
	if _, err := SafeJoin(dir, "../escape"); err == nil {
		t.Fatal("expected traversal error")
	}
	_ = os.WriteFile(filepath.Join(dir, "x"), []byte("1"), 0o600)
}

func TestSelectFiles(t *testing.T) {
	files := []FileMeta{
		{ID: "1", OriginalName: "a.txt"},
		{ID: "2", OriginalName: "b.txt"},
	}
	got, err := SelectFiles(files, nil, false)
	if err == nil {
		t.Fatal("multi without select should error")
	}
	got, err = SelectFiles(files, []string{"b.txt"}, false)
	if err != nil || len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("%v %v", got, err)
	}
	got, err = SelectFiles(files, nil, true)
	if err != nil || len(got) != 2 {
		t.Fatal(got, err)
	}
}
