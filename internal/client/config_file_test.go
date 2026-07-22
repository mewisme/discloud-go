package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	// Force config dir under temp: on Windows APPDATA=dir → dir/discloud
	// on Unix XDG_CONFIG_HOME=dir → dir/discloud

	path, base, origin, err := SaveConfigFile("http://api.example/", "http://app.example/")
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(dir, "discloud", "config.json")
	if path != wantPath {
		t.Fatalf("path=%q want %q", path, wantPath)
	}
	if base != "http://api.example" || origin != "http://app.example" {
		t.Fatalf("saved %q %q", base, origin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f configFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if f.Base != "http://api.example" || f.Origin != "http://app.example" {
		t.Fatalf("%+v", f)
	}

	// Merge: only update base
	_, base2, origin2, err := SaveConfigFile("http://api2.example", "")
	if err != nil {
		t.Fatal(err)
	}
	if base2 != "http://api2.example" || origin2 != "http://app.example" {
		t.Fatalf("merge got %q %q", base2, origin2)
	}
}
