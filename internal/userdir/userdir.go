// Package userdir provides platform-aware DisCloud user directories.
package userdir

import (
	"os"
	"path/filepath"
	"runtime"
)

// ConfigDir is the user configuration directory (client config.yaml, cookies).
// Windows: %APPDATA%\discloud
// Unix: $XDG_CONFIG_HOME/discloud or ~/.config/discloud
func ConfigDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(base, "discloud")
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "discloud")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "discloud")
}

// DataDir is the persistent data directory (SQLite DB, secrets, run, logs).
// Windows: %LOCALAPPDATA%\discloud
// Unix: $XDG_DATA_HOME/discloud or ~/.local/share/discloud
func DataDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		return filepath.Join(base, "discloud")
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "discloud")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "discloud")
}

// InstallDir is where CLI and server versioned binaries live.
// Windows: %LOCALAPPDATA%\discloud
// Unix: ~/.discloud
func InstallDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		return filepath.Join(base, "discloud")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".discloud")
}

// ServerInstallDir is <InstallDir>/server (versions, current, config).
func ServerInstallDir() string {
	return filepath.Join(InstallDir(), "server")
}

// ServerConfigDir is the canonical server configuration directory.
func ServerConfigDir() string {
	return filepath.Join(ServerInstallDir(), "config")
}

// ServerCurrentDir is the active server presentation directory.
func ServerCurrentDir() string {
	return filepath.Join(ServerInstallDir(), "current")
}

// ServerVersionsDir holds versioned server binaries.
func ServerVersionsDir() string {
	return filepath.Join(ServerInstallDir(), "versions")
}

// RuntimeDir holds PID metadata.
func RuntimeDir() string {
	return filepath.Join(DataDir(), "run")
}

// LogDir holds server logs.
func LogDir() string {
	return filepath.Join(DataDir(), "logs")
}

// DatabasePath is the default local SQLite database path.
func DatabasePath() string {
	return filepath.Join(DataDir(), "discloud.db")
}
