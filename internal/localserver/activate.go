package localserver

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mewisme/discloud-go/internal/serverconfig"
)

// Activate makes versions/<version> the binary current/ presents and refreshes
// the config presentation beside it. Unix symlinks; Windows copies.
func Activate(p Paths, version string) error {
	src := p.VersionBinary(version)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("activate %s: %w", version, err)
	}
	if err := os.MkdirAll(p.CurrentDir(), 0o700); err != nil {
		return err
	}
	dst := p.CurrentBinary()
	_ = os.Remove(dst)
	if runtime.GOOS == "windows" {
		if err := copyFile(src, dst, 0o700); err != nil {
			return err
		}
	} else if err := os.Symlink(src, dst); err != nil {
		return err
	}
	if err := os.WriteFile(p.versionMarker(), []byte(version+"\n"), 0o600); err != nil {
		return err
	}
	return serverconfig.SyncCurrent(p.InstallDir)
}

// ActiveVersion is the version current/ presents, or "" when nothing is active.
func ActiveVersion(p Paths) string {
	if data, err := os.ReadFile(p.versionMarker()); err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			return v
		}
	}
	// Fallback for a symlinked install whose marker was lost.
	if target, err := os.Readlink(p.CurrentBinary()); err == nil {
		return filepath.Base(filepath.Dir(target))
	}
	return ""
}

func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// moveFile renames, falling back to copy+delete when src and dst are on
// different volumes (temp dir vs install dir).
func moveFile(src, dst string, mode os.FileMode) error {
	if err := os.Rename(src, dst); err == nil {
		return os.Chmod(dst, mode)
	}
	if err := copyFile(src, dst, mode); err != nil {
		return err
	}
	return os.Remove(src)
}
