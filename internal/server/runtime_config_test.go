package server

import (
	"io"
	"net/http"
	"testing"
)

func TestAdminConfigRequiresPostgres(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "cfgadmin", "password1")

	res := e.do(t, http.MethodGet, "/api/admin/config", cookie, "", "")
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501: %s", res.StatusCode, body)
	}
}

func TestEffectiveChunkSizeDefault(t *testing.T) {
	e := newAuthEnv(t)
	if got := e.srv.effectiveChunkSize(); got != defaultChunkSize {
		t.Fatalf("chunk = %d, want %d", got, defaultChunkSize)
	}
}
