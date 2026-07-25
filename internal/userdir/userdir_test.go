package userdir

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigDirContainsDiscloud(t *testing.T) {
	d := ConfigDir()
	if !strings.Contains(filepath.ToSlash(d), "discloud") {
		t.Fatalf("ConfigDir=%q", d)
	}
}

func TestDataDirAndInstallLayout(t *testing.T) {
	if !strings.HasSuffix(filepath.Clean(ServerInstallDir()), filepath.Join("discloud", "server")) &&
		!strings.HasSuffix(filepath.Clean(ServerInstallDir()), "discloud"+string(filepath.Separator)+"server") {
		// Soft check: ends with discloud/server
		if !strings.Contains(filepath.ToSlash(ServerInstallDir()), "discloud/server") {
			t.Fatalf("ServerInstallDir=%q", ServerInstallDir())
		}
	}
	if DatabasePath() == "" || !strings.HasSuffix(DatabasePath(), "discloud.db") {
		t.Fatalf("DatabasePath=%q", DatabasePath())
	}
	if runtime.GOOS == "windows" {
		if DataDir() == "" {
			t.Fatal("empty DataDir")
		}
	}
}
