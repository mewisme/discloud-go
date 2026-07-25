// Package localserver installs, activates, and supervises the DisCloud server
// as a detached user process. No OS services are involved: lifecycle state is a
// PID file under the data directory.
package localserver

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mewisme/discloud-go/internal/serverconfig"
	"github.com/mewisme/discloud-go/internal/userdir"
)

// binaryStem is the installed executable name; release archives ship "discloud"
// and it is renamed on install so it never collides with the CLI.
const binaryStem = "discloud-server"

// Paths locates every directory the local server uses. Use DefaultPaths in
// production; set both fields explicitly in tests.
type Paths struct {
	InstallDir string // …/discloud/server — versions/, current/, config/
	DataDir    string // db, secrets, run/, logs/
}

// DefaultPaths resolves the per-user layout from userdir.
func DefaultPaths() Paths {
	return Paths{InstallDir: userdir.ServerInstallDir(), DataDir: userdir.DataDir()}
}

func (p Paths) VersionsDir() string { return filepath.Join(p.InstallDir, "versions") }
func (p Paths) CurrentDir() string  { return serverconfig.CurrentDir(p.InstallDir) }
func (p Paths) ConfigDir() string   { return serverconfig.Dir(p.InstallDir) }
func (p Paths) ConfigPath() string  { return serverconfig.Path(p.InstallDir) }
func (p Paths) RunDir() string      { return filepath.Join(p.DataDir, "run") }
func (p Paths) PIDPath() string     { return filepath.Join(p.RunDir(), "discloud.pid") }
func (p Paths) LogDir() string      { return filepath.Join(p.DataDir, "logs") }
func (p Paths) LogPath() string     { return filepath.Join(p.LogDir(), "server.log") }
func (p Paths) DatabasePath() string {
	return filepath.Join(p.DataDir, "discloud.db")
}

// VersionDir is versions/<vX.Y.Z>. Version must already be validated.
func (p Paths) VersionDir(version string) string {
	return filepath.Join(p.VersionsDir(), version)
}

// VersionBinary is the executable inside versions/<vX.Y.Z>.
func (p Paths) VersionBinary(version string) string {
	return filepath.Join(p.VersionDir(version), BinaryName())
}

// CurrentBinary is the activated executable (symlink on Unix, copy on Windows).
func (p Paths) CurrentBinary() string {
	return filepath.Join(p.CurrentDir(), BinaryName())
}

// versionMarker records which version current/ presents; needed on Windows
// where activation copies instead of symlinking.
func (p Paths) versionMarker() string {
	return filepath.Join(p.CurrentDir(), ".version")
}

// BinaryName is the server executable filename for this platform.
func BinaryName() string {
	if runtime.GOOS == "windows" {
		return binaryStem + ".exe"
	}
	return binaryStem
}

// NormalizeVersion trims and forces a leading "v". It rejects anything that
// could escape versions/ since the value comes from a user flag.
func NormalizeVersion(version string) (string, error) {
	v := strings.TrimSpace(version)
	if v == "" {
		return "", fmt.Errorf("version is empty")
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if v != filepath.Base(v) || strings.ContainsAny(v, `/\`) || strings.Contains(v, "..") {
		return "", fmt.Errorf("invalid version %q", version)
	}
	return v, nil
}
