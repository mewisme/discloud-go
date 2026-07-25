package localserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTailReturnsLastLines(t *testing.T) {
	p := testPaths(t)
	if lines, err := Tail(p, 5); err != nil || lines != nil {
		t.Fatalf("missing log should be empty: %v %v", lines, err)
	}
	if err := os.MkdirAll(p.LogDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	if err := os.WriteFile(p.LogPath(), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := Tail(p, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 || lines[0] != "line 7" || lines[2] != "line 9" {
		t.Fatalf("tail = %v", lines)
	}
}

func TestFormatLogLine(t *testing.T) {
	t.Parallel()
	in := `{"time":"2026-07-25T12:34:56.789Z","level":"INFO","msg":"request","method":"GET","path":"/readyz","status":200,"duration_ms":3}`
	got := FormatLogLine(in)
	if !strings.Contains(got, "INFO") || !strings.Contains(got, "request") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "method=GET") || !strings.Contains(got, "status=200") {
		t.Fatalf("attrs missing: %q", got)
	}
	if strings.Contains(got, "{") {
		t.Fatalf("still JSON: %q", got)
	}

	plain := FormatLogLine("not json at all")
	if plain != "not json at all" {
		t.Fatalf("plain = %q", plain)
	}

	fakeTok := strings.Repeat("A", 24) + "." + strings.Repeat("B", 6) + "." + strings.Repeat("C", 27)
	sec := FormatLogLine(`{"msg":"boot","token":"` + fakeTok + `"}`)
	if strings.Contains(sec, fakeTok) {
		t.Fatalf("secret leaked: %q", sec)
	}
}

func TestRedactHidesSecrets(t *testing.T) {
	// Built at runtime so secret scanners do not flag a Discord-shaped literal.
	fakeTok := strings.Repeat("A", 24) + "." + strings.Repeat("B", 6) + "." + strings.Repeat("C", 27)
	in := `{"msg":"boot","token":"` + fakeTok + `","database_url":"postgres://u:p@h/db"}`
	out := Redact(in)
	if strings.Contains(out, fakeTok) || strings.Contains(out, strings.Repeat("C", 27)) {
		t.Fatalf("token leaked: %s", out)
	}
	if !strings.Contains(out, "boot") {
		t.Fatalf("redaction destroyed the message: %s", out)
	}
	if got := Redact("DISCLOUD_BOT_TOKEN=supersecretvalue"); strings.Contains(got, "supersecretvalue") {
		t.Fatalf("env-style secret leaked: %s", got)
	}
}

func TestRedactedTailUsedInStartFailure(t *testing.T) {
	p := testPaths(t)
	if err := os.MkdirAll(p.LogDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.LogPath(), []byte("token=abc123def456\nready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := RedactedTail(p, 10)
	if strings.Contains(got, "abc123def456") || !strings.Contains(got, "ready") {
		t.Fatalf("RedactedTail = %q", got)
	}
}

func TestStatusReportsStaleAndReady(t *testing.T) {
	p := testPaths(t)
	s := GetStatus(context.Background(), p, nil)
	if s.Installed || s.Running || s.StalePID {
		t.Fatalf("empty install status = %+v", s)
	}

	writePIDFile(t, p, PIDInfo{PID: deadPID(t), StartedAt: time.Now().UTC()})
	if s := GetStatus(context.Background(), p, nil); !s.StalePID || s.Running {
		t.Fatalf("stale status = %+v", s)
	}

	// A live process plus a reachable /readyz reports ready.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if err := saveProbeConfig(p, srv.URL); err != nil {
		t.Fatal(err)
	}
	writePIDFile(t, p, PIDInfo{PID: os.Getpid(), StartedAt: time.Now().UTC(), Version: "v1.0.0"})
	s = GetStatus(context.Background(), p, srv.Client())
	if !s.Running || !s.Ready {
		t.Fatalf("ready status = %+v (%s)", s, s.ReadyError)
	}
	if s.Version != "v1.0.0" {
		t.Fatalf("version = %q", s.Version)
	}
}
