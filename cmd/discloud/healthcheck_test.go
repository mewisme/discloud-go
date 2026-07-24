package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunHealthcheck(t *testing.T) {
	ready := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		if !ready {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	}))
	defer ts.Close()

	_, port, err := net.SplitHostPort(ts.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DISCLOUD_PORT", port)

	if err := runHealthcheck(); err == nil {
		t.Fatal("expected error when not ready")
	}
	ready = true
	if err := runHealthcheck(); err != nil {
		t.Fatalf("ready: %v", err)
	}
}
