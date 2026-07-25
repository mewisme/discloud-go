package localserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mewisme/discloud-go/internal/config"
	"github.com/mewisme/discloud-go/internal/serverconfig"
)

const (
	defaultStartTimeout = 45 * time.Second
	defaultStopTimeout  = 20 * time.Second
	pollInterval        = 250 * time.Millisecond
	defaultPort         = "6373"
)

// PIDInfo is the JSON stored in run/discloud.pid.
type PIDInfo struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
	Binary    string    `json:"binary"`
	Version   string    `json:"version"`
}

// StartOptions tunes readiness polling. The zero value is fine.
type StartOptions struct {
	Timeout time.Duration
	HTTP    *http.Client
}

// ReadPID returns the recorded process metadata. Missing file → os.ErrNotExist.
func ReadPID(p Paths) (PIDInfo, error) {
	data, err := os.ReadFile(p.PIDPath())
	if err != nil {
		return PIDInfo{}, err
	}
	var info PIDInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return PIDInfo{}, fmt.Errorf("invalid %s: %w", p.PIDPath(), err)
	}
	return info, nil
}

func writePID(p Paths, info PIDInfo) error {
	if err := os.MkdirAll(p.RunDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.PIDPath(), data, 0o600)
}

// Running reports the live server process. A PID file whose process is gone is
// stale: it gets removed and ok is false.
func Running(p Paths) (PIDInfo, bool) {
	info, err := ReadPID(p)
	if err != nil {
		return PIDInfo{}, false
	}
	if !processAlive(info.PID) {
		_ = os.Remove(p.PIDPath())
		return info, false
	}
	return info, true
}

// Start launches the activated server detached, records its PID, and waits for
// /readyz. On failure it stops the process and reports a redacted log tail.
func Start(ctx context.Context, p Paths, opts StartOptions) (PIDInfo, error) {
	if info, ok := Running(p); ok {
		return info, fmt.Errorf("server already running (pid %d)", info.PID)
	}
	binary := p.CurrentBinary()
	if _, err := os.Stat(binary); err != nil {
		return PIDInfo{}, fmt.Errorf("no active server binary at %s: run install first", binary)
	}
	for _, dir := range []string{p.DataDir, p.RunDir(), p.LogDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return PIDInfo{}, err
		}
	}
	logFile, err := os.OpenFile(p.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return PIDInfo{}, err
	}
	defer logFile.Close()

	configDir, err := filepath.Abs(p.CurrentDir())
	if err != nil {
		return PIDInfo{}, err
	}
	dataDir, err := filepath.Abs(p.DataDir)
	if err != nil {
		return PIDInfo{}, err
	}

	cmd := exec.Command(binary)
	cmd.Dir = dataDir
	cmd.Env = append(os.Environ(),
		"DISCLOUD_CONFIG_DIR="+configDir,
		"DISCLOUD_DATA_DIR="+dataDir,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = detachAttr()

	if err := cmd.Start(); err != nil {
		return PIDInfo{}, fmt.Errorf("start %s: %w", binary, err)
	}
	info := PIDInfo{
		PID:       cmd.Process.Pid,
		StartedAt: time.Now().UTC(),
		Binary:    binary,
		Version:   ActiveVersion(p),
	}
	// Detach: the server outlives this CLI invocation.
	_ = cmd.Process.Release()

	if err := writePID(p, info); err != nil {
		return info, err
	}
	if err := waitReady(ctx, p, info.PID, opts); err != nil {
		_ = Stop(ctx, p, defaultStopTimeout)
		return info, fmt.Errorf("%w\n--- %s (redacted) ---\n%s", err, p.LogPath(), RedactedTail(p, 20))
	}
	return info, nil
}

// Stop terminates the running server and clears the PID file. Stopping an
// already-stopped server is not an error.
func Stop(ctx context.Context, p Paths, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultStopTimeout
	}
	info, ok := Running(p)
	if !ok {
		_ = os.Remove(p.PIDPath())
		return nil
	}
	if err := terminate(info.PID); err != nil && processAlive(info.PID) {
		return fmt.Errorf("stop pid %d: %w", info.PID, err)
	}
	if waitGone(ctx, info.PID, timeout) {
		_ = os.Remove(p.PIDPath())
		return nil
	}
	if err := forceKill(info.PID); err != nil && processAlive(info.PID) {
		return fmt.Errorf("kill pid %d: %w", info.PID, err)
	}
	if !waitGone(ctx, info.PID, 5*time.Second) {
		return fmt.Errorf("server pid %d did not exit", info.PID)
	}
	_ = os.Remove(p.PIDPath())
	return nil
}

// Restart stops the server if it is running, then starts it again.
func Restart(ctx context.Context, p Paths, opts StartOptions) (PIDInfo, error) {
	if err := Stop(ctx, p, defaultStopTimeout); err != nil {
		return PIDInfo{}, err
	}
	return Start(ctx, p, opts)
}

func waitGone(ctx context.Context, pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		select {
		case <-ctx.Done():
			return !processAlive(pid)
		case <-time.After(100 * time.Millisecond):
		}
	}
	return !processAlive(pid)
}

func waitReady(ctx context.Context, p Paths, pid int, opts StartOptions) error {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultStartTimeout
	}
	hc := opts.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 3 * time.Second}
	}
	url, err := ProbeURL(p, "/readyz")
	if err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if !processAlive(pid) {
			return fmt.Errorf("server exited during startup (pid %d)", pid)
		}
		if err := probeOnce(ctx, hc, url); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("server did not become ready at %s within %s: %v", url, timeout, lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func probeOnce(ctx context.Context, hc *http.Client, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer drain(res)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, res.Status)
	}
	return nil
}

// ProbeURL builds the local health URL from canonical server.yaml.
func ProbeURL(p Paths, path string) (string, error) {
	f, err := serverconfig.Load(p.InstallDir)
	if err != nil {
		return "", err
	}
	port := f.Port
	if port == "" {
		port = defaultPort
	}
	return config.ProbeURL(f.APIURL, f.Host, port, path), nil
}
