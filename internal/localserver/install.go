package localserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mewisme/discloud-go/internal/client"
	"github.com/mewisme/discloud-go/internal/config"
	"github.com/mewisme/discloud-go/internal/serverconfig"
)

// Options configures install/update/uninstall. Zero Paths means the per-user
// default layout.
type Options struct {
	Paths  Paths
	Source Source

	// SyncClientConfig points the CLI at this server when config.yaml has no
	// base/origin yet.
	SyncClientConfig bool
}

func (o Options) paths() Paths {
	if o.Paths.InstallDir == "" || o.Paths.DataDir == "" {
		return DefaultPaths()
	}
	return o.Paths
}

// InstallResult describes what an install left on disk.
type InstallResult struct {
	Version       string
	BinaryPath    string
	ConfigPath    string
	ConfigCreated bool
}

// Install downloads, verifies, and activates a release. It never starts the
// server and never overwrites existing config.
func Install(ctx context.Context, o Options, version string) (InstallResult, error) {
	p := o.paths()

	ver, err := o.Source.ResolveVersion(ctx, version)
	if err != nil {
		return InstallResult{}, err
	}

	tmp, err := os.MkdirTemp("", "discloud-server-")
	if err != nil {
		return InstallResult{}, err
	}
	defer os.RemoveAll(tmp)

	archive, err := o.Source.Download(ctx, ver, tmp)
	if err != nil {
		return InstallResult{}, err
	}
	stage := filepath.Join(tmp, "stage")
	if err := Extract(archive, stage); err != nil {
		return InstallResult{}, err
	}
	staged, err := RenameServerBinary(stage)
	if err != nil {
		return InstallResult{}, err
	}

	dest := p.VersionDir(ver)
	if err := os.RemoveAll(dest); err != nil {
		return InstallResult{}, err
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return InstallResult{}, err
	}
	binary := p.VersionBinary(ver)
	if err := moveFile(staged, binary, 0o700); err != nil {
		return InstallResult{}, err
	}

	created, err := EnsureConfig(p)
	if err != nil {
		return InstallResult{}, err
	}

	previous := ActiveVersion(p)
	if err := Activate(p, ver); err != nil {
		if previous != "" && previous != ver {
			_ = Activate(p, previous)
		}
		return InstallResult{}, err
	}

	res := InstallResult{Version: ver, BinaryPath: binary, ConfigPath: p.ConfigPath(), ConfigCreated: created}
	if o.SyncClientConfig {
		if err := syncClientConfig(p); err != nil {
			return res, fmt.Errorf("install succeeded but CLI config sync failed: %w", err)
		}
	}
	return res, nil
}

// EnsureConfig writes canonical server.yaml with local defaults when absent.
// An existing server.yaml (and .env in every case) is left untouched.
func EnsureConfig(p Paths) (bool, error) {
	if err := os.MkdirAll(p.DataDir, 0o700); err != nil {
		return false, err
	}
	if _, err := os.Stat(p.ConfigPath()); err == nil {
		return false, serverconfig.SyncCurrent(p.InstallDir)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	dsn := config.CanonicalSQLiteDSN(p.DatabasePath())
	return true, serverconfig.Save(p.InstallDir, serverconfig.DefaultLocal(dsn))
}

// UpdateResult reports what changed and whether the server was brought back up.
type UpdateResult struct {
	InstallResult
	PreviousVersion string
	Restarted       bool
}

// Update installs a new version, keeping config and data. A running server is
// stopped first and restarted only if it was running.
func Update(ctx context.Context, o Options, version string) (UpdateResult, error) {
	p := o.paths()
	previous := ActiveVersion(p)
	_, wasRunning := Running(p)

	if wasRunning {
		if err := Stop(ctx, p, defaultStopTimeout); err != nil {
			return UpdateResult{PreviousVersion: previous}, err
		}
	}

	res, err := Install(ctx, o, version)
	if err != nil {
		out := UpdateResult{PreviousVersion: previous}
		if previous != "" {
			_ = Activate(p, previous)
		}
		if wasRunning {
			if _, serr := Start(ctx, p, StartOptions{}); serr == nil {
				out.Restarted = true
			}
		}
		return out, err
	}

	out := UpdateResult{InstallResult: res, PreviousVersion: previous}
	if wasRunning {
		if _, err := Start(ctx, p, StartOptions{}); err != nil {
			return out, fmt.Errorf("installed %s but restart failed: %w", res.Version, err)
		}
		out.Restarted = true
	}
	return out, nil
}

// Uninstall stops the server and removes the installed binaries. Without purge
// the canonical config, database, logs, and secrets survive.
func Uninstall(ctx context.Context, o Options, purge bool) error {
	p := o.paths()
	if _, running := Running(p); running {
		if err := Stop(ctx, p, defaultStopTimeout); err != nil {
			return err
		}
	}
	_ = os.Remove(p.PIDPath())
	if err := os.RemoveAll(p.VersionsDir()); err != nil {
		return err
	}
	if err := os.Remove(p.CurrentBinary()); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(p.versionMarker()); err != nil && !os.IsNotExist(err) {
		return err
	}
	if !purge {
		return nil
	}

	if err := os.RemoveAll(p.InstallDir); err != nil {
		return err
	}
	if err := os.RemoveAll(p.LogDir()); err != nil {
		return err
	}
	if err := os.RemoveAll(p.RunDir()); err != nil {
		return err
	}
	db := p.DatabasePath()
	for _, f := range []string{
		db, db + "-wal", db + "-shm",
		filepath.Join(p.DataDir, config.VisitorHashSaltFile),
		filepath.Join(p.DataDir, config.AppSecretFile),
		filepath.Join(p.DataDir, ".visitor_hash_salt"), // pre-rename salt
	} {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// syncClientConfig fills only the CLI fields that are still empty.
func syncClientConfig(p Paths) error {
	cur, err := client.LoadFileConfig()
	if err != nil {
		return err
	}
	if cur.Base != "" && cur.Origin != "" {
		return nil
	}
	f, err := serverconfig.Load(p.InstallDir)
	if err != nil {
		return err
	}
	base, origin := f.APIURL, f.CORS
	if cur.Base != "" {
		base = ""
	}
	if cur.Origin != "" {
		origin = ""
	}
	if base == "" && origin == "" {
		return nil
	}
	_, _, _, err = client.SaveConfigFile(base, origin)
	return err
}
