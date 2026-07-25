package localserver

import (
	"context"
	"net/http"
	"os"
	"time"
)

// Status is a point-in-time view of the local server for `discloud server status`.
type Status struct {
	Installed  bool
	Version    string
	BinaryPath string
	ConfigPath string
	LogPath    string
	DataDir    string

	Running   bool
	PID       int
	StartedAt time.Time
	StalePID  bool // a PID file existed but its process is gone

	ProbeURL   string
	Ready      bool
	ReadyError string
}

// GetStatus inspects the install, the PID file, and /readyz. It never fails:
// problems surface as fields on Status.
func GetStatus(ctx context.Context, p Paths, hc *http.Client) Status {
	s := Status{
		BinaryPath: p.CurrentBinary(),
		ConfigPath: p.ConfigPath(),
		LogPath:    p.LogPath(),
		DataDir:    p.DataDir,
		Version:    ActiveVersion(p),
	}
	if _, err := os.Stat(s.BinaryPath); err == nil {
		s.Installed = true
	}

	_, pidErr := ReadPID(p)
	info, running := Running(p)
	s.Running = running
	s.StalePID = pidErr == nil && !running
	if running || s.StalePID {
		s.PID = info.PID
		s.StartedAt = info.StartedAt
		if info.Version != "" {
			s.Version = info.Version
		}
	}

	url, err := ProbeURL(p, "/readyz")
	if err != nil {
		s.ReadyError = err.Error()
		return s
	}
	s.ProbeURL = url
	if !running {
		return s
	}
	if hc == nil {
		hc = &http.Client{Timeout: 3 * time.Second}
	}
	if err := probeOnce(ctx, hc, url); err != nil {
		s.ReadyError = err.Error()
		return s
	}
	s.Ready = true
	return s
}
