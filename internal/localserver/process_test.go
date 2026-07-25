package localserver

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/mewisme/discloud-go/internal/serverconfig"
)

// deadPID starts and reaps a throwaway child so its PID is certainly free.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	for i := 0; i < 200 && processAlive(pid); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	return pid
}

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("this process should be alive")
	}
	if processAlive(0) || processAlive(-5) {
		t.Fatal("invalid pids must not be alive")
	}
	if pid := deadPID(t); processAlive(pid) {
		t.Fatalf("pid %d should have exited", pid)
	}
}

func writePIDFile(t *testing.T, p Paths, info PIDInfo) {
	t.Helper()
	if err := writePID(p, info); err != nil {
		t.Fatal(err)
	}
}

func TestRunningDetectsStalePIDFile(t *testing.T) {
	p := testPaths(t)
	stale := PIDInfo{PID: deadPID(t), StartedAt: time.Now().UTC(), Binary: p.CurrentBinary(), Version: "v1.0.0"}
	writePIDFile(t, p, stale)

	if _, err := ReadPID(p); err != nil {
		t.Fatalf("ReadPID: %v", err)
	}
	info, ok := Running(p)
	if ok {
		t.Fatal("stale pid reported as running")
	}
	if info.Version != "v1.0.0" {
		t.Fatalf("stale metadata lost: %+v", info)
	}
	if _, err := os.Stat(p.PIDPath()); !os.IsNotExist(err) {
		t.Fatal("stale pid file should be removed")
	}
}

func TestRunningTrueForLiveProcessAndPIDJSONShape(t *testing.T) {
	p := testPaths(t)
	writePIDFile(t, p, PIDInfo{PID: os.Getpid(), StartedAt: time.Now().UTC(), Binary: "x", Version: "v2"})

	if _, ok := Running(p); !ok {
		t.Fatal("live pid should be running")
	}
	data, err := os.ReadFile(p.PIDPath())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"pid", "startedAt", "binary", "version"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("pid file missing %q: %s", key, data)
		}
	}
}

func TestStopWithoutPIDFileIsNoOp(t *testing.T) {
	if err := Stop(context.Background(), testPaths(t), time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestProbeURL(t *testing.T) {
	p := testPaths(t)
	// No config yet → localhost default port.
	got, err := ProbeURL(p, "/readyz")
	if err != nil || got != "http://127.0.0.1:6373/readyz" {
		t.Fatalf("default probe = %q, %v", got, err)
	}

	// A wildcard bind is probed on the loopback address.
	if err := serverconfig.Save(p.InstallDir, serverconfig.File{Host: "0.0.0.0", Port: "6400"}); err != nil {
		t.Fatal(err)
	}
	got, err = ProbeURL(p, "/readyz")
	if err != nil || got != "http://127.0.0.1:6400/readyz" {
		t.Fatalf("wildcard probe = %q, %v", got, err)
	}
}

func TestStartWithoutInstallFails(t *testing.T) {
	_, err := Start(context.Background(), testPaths(t), StartOptions{})
	if err == nil {
		t.Fatal("expected error when no binary is active")
	}
}
