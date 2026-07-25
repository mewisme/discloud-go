package localserver

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/mewisme/discloud-go/internal/config"
	"github.com/mewisme/discloud-go/internal/serverconfig"
)

// InstallOptions is the CLI-facing install/update input.
type InstallOptions struct {
	Version   string
	Yes       bool
	BotTokens []string
	ChannelID string
	GuildID   string
	ChunkSize int64
	CORS      string
}

// UninstallOptions is the CLI-facing uninstall input.
type UninstallOptions struct {
	Purge bool
	Yes   bool
}

// CLIInstallResult is returned to the CLI after install/update.
type CLIInstallResult struct {
	Version    string   `json:"version"`
	Binary     string   `json:"binary"`
	ConfigPath string   `json:"configPath"`
	APIURL     string   `json:"apiUrl"`
	CORS       string   `json:"cors"`
	Restarted  bool     `json:"restarted,omitempty"`
	Messages   []string `json:"messages,omitempty"`
}

// CLIStatus is the rich status document for `discloud server status`.
type CLIStatus struct {
	Installed    bool   `json:"installed"`
	Version      string `json:"version"`
	Binary       string `json:"binary"`
	ConfigPath   string `json:"configPath"`
	EnvPath      string `json:"envPath"`
	DataDir      string `json:"dataDir"`
	DatabasePath string `json:"databasePath"`
	LogPath      string `json:"logPath"`
	ProcessState string `json:"processState"`
	PID          int    `json:"pid,omitempty"`
	HealthState  string `json:"healthState"`
	ReadyState   string `json:"readyState"`
	APIURL       string `json:"apiUrl"`
	StoreBackend string `json:"store"`
	CacheBackend string `json:"cache"`
}

// Masked returns a copy safe for display (URLs already non-secret for sqlite).
func (s CLIStatus) Masked() CLIStatus { return s }

// VersionInfo is returned by `discloud server version`.
type VersionInfo struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	Binary    string `json:"binary"`
}

// InstallCLI runs Install with CLI options and applies bot/channel if provided.
func InstallCLI(ctx context.Context, opts InstallOptions) (CLIInstallResult, error) {
	p := DefaultPaths()
	o := Options{Paths: p, SyncClientConfig: true}
	res, err := Install(ctx, o, opts.Version)
	if err != nil {
		return CLIInstallResult{}, err
	}
	if err := applyInstallSecrets(p, opts); err != nil {
		return CLIInstallResult{}, err
	}
	f, _ := serverconfig.Load(p.InstallDir)
	return CLIInstallResult{
		Version:    res.Version,
		Binary:     p.CurrentBinary(),
		ConfigPath: res.ConfigPath,
		APIURL:     f.APIURL,
		CORS:       f.CORS,
	}, nil
}

// UpdateCLI runs Update with CLI options.
func UpdateCLI(ctx context.Context, opts InstallOptions) (CLIInstallResult, error) {
	p := DefaultPaths()
	res, err := Update(ctx, Options{Paths: p}, opts.Version)
	if err != nil {
		return CLIInstallResult{Restarted: res.Restarted}, err
	}
	f, _ := serverconfig.Load(p.InstallDir)
	return CLIInstallResult{
		Version:    res.Version,
		Binary:     p.CurrentBinary(),
		ConfigPath: res.ConfigPath,
		APIURL:     f.APIURL,
		CORS:       f.CORS,
		Restarted:  res.Restarted,
	}, nil
}

// UninstallCLI runs Uninstall and returns human messages.
func UninstallCLI(ctx context.Context, opts UninstallOptions) (CLIInstallResult, error) {
	p := DefaultPaths()
	if err := Uninstall(ctx, Options{Paths: p}, opts.Purge); err != nil {
		return CLIInstallResult{}, err
	}
	msg := []string{"Removed server binaries and version directories."}
	if opts.Purge {
		msg = append(msg, "Purged config, database, logs, and secrets.")
	} else {
		msg = append(msg, "Preserved server.yaml, .env, database, logs, and secrets.")
	}
	return CLIInstallResult{Messages: msg}, nil
}

func applyInstallSecrets(p Paths, opts InstallOptions) error {
	if len(opts.BotTokens) == 0 && opts.ChannelID == "" && opts.GuildID == "" && opts.ChunkSize == 0 && opts.CORS == "" {
		return nil
	}
	f, err := serverconfig.Load(p.InstallDir)
	if err != nil {
		return err
	}
	if len(opts.BotTokens) > 0 {
		f.BotTokens = opts.BotTokens
	}
	if opts.ChannelID != "" {
		f.ChannelID = opts.ChannelID
	}
	if opts.GuildID != "" {
		f.GuildID = opts.GuildID
	}
	if opts.ChunkSize > 0 {
		f.ChunkSize = opts.ChunkSize
	}
	if opts.CORS != "" {
		f.CORS = opts.CORS
	}
	return serverconfig.Save(p.InstallDir, f)
}

// StartDefault starts the server with default paths.
func StartDefault(ctx context.Context) (CLIStatus, error) {
	p := DefaultPaths()
	info, err := Start(ctx, p, StartOptions{})
	if err != nil {
		return CLIStatus{}, err
	}
	st := buildCLIStatus(ctx, p)
	st.PID = info.PID
	return st, nil
}

// StopDefault stops the server with default paths.
func StopDefault(ctx context.Context) error {
	return Stop(ctx, DefaultPaths(), defaultStopTimeout)
}

// RestartDefault stops then starts.
func RestartDefault(ctx context.Context) (CLIStatus, error) {
	p := DefaultPaths()
	_ = Stop(ctx, p, defaultStopTimeout)
	info, err := Start(ctx, p, StartOptions{})
	if err != nil {
		return CLIStatus{}, err
	}
	st := buildCLIStatus(ctx, p)
	st.PID = info.PID
	return st, nil
}

// GetStatusDefault returns CLI status for default paths.
func GetStatusDefault(ctx context.Context) (CLIStatus, error) {
	return buildCLIStatus(ctx, DefaultPaths()), nil
}

func buildCLIStatus(ctx context.Context, p Paths) CLIStatus {
	raw := GetStatus(ctx, p, nil)
	f, _ := serverconfig.Load(p.InstallDir)
	storeName, cacheName := "sqlite", "memory"
	if f.DatabaseURL != "" {
		if info, err := config.ParseDatabaseURL(f.DatabaseURL); err == nil {
			storeName = string(info.Backend)
		}
	}
	if strings.TrimSpace(f.ValkeyURL) != "" {
		cacheName = "valkey"
	}
	process := "stopped"
	health := "unknown"
	ready := "not ready"
	if !raw.Installed {
		process = "not installed"
	} else if raw.StalePID {
		process = "stale pid"
	} else if raw.Running {
		process = "running"
		if raw.Ready {
			health = "ok"
			ready = "ready"
		} else if raw.ReadyError != "" {
			health = "unreachable"
			ready = "not ready"
		} else {
			health = "responding"
			ready = "not ready"
		}
	}
	api := f.APIURL
	if api == "" {
		api = raw.ProbeURL
	}
	return CLIStatus{
		Installed:    raw.Installed,
		Version:      raw.Version,
		Binary:       raw.BinaryPath,
		ConfigPath:   raw.ConfigPath,
		EnvPath:      serverconfig.EnvPath(p.InstallDir),
		DataDir:      raw.DataDir,
		DatabasePath: p.DatabasePath(),
		LogPath:      raw.LogPath,
		ProcessState: process,
		PID:          raw.PID,
		HealthState:  health,
		ReadyState:   ready,
		APIURL:       api,
		StoreBackend: storeName,
		CacheBackend: cacheName,
	}
}

// VersionInfoDefault returns installed version metadata.
func VersionInfoDefault() (VersionInfo, error) {
	p := DefaultPaths()
	bin := p.CurrentBinary()
	_, err := os.Stat(bin)
	installed := err == nil
	return VersionInfo{
		Installed: installed,
		Version:   ActiveVersion(p),
		Binary:    bin,
	}, nil
}

// LogPathDefault returns the server log path.
func LogPathDefault() string { return DefaultPaths().LogPath() }

// TailLogDefault returns the last n log lines.
func TailLogDefault(n int) ([]string, error) { return Tail(DefaultPaths(), n) }

// PrintLogsDefault prints or follows logs in human-readable form.
// When follow is true, streams until ctx is cancelled (Ctrl+C).
func PrintLogsDefault(ctx context.Context, w io.Writer, n int, follow bool) error {
	p := DefaultPaths()
	if !follow {
		lines, err := Tail(p, n)
		if err != nil {
			return err
		}
		for _, line := range lines {
			if err := writeLogLine(w, line); err != nil {
				return err
			}
		}
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return Follow(ctx, p, n, w)
}
