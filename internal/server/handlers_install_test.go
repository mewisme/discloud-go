package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInstallScriptsBakeEnv(t *testing.T) {
	srv := &Server{
		publicBaseURL: "https://api.example.com",
		webOrigin:     "https://app.example.com",
	}

	t.Run("sh", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
		rec := httptest.NewRecorder()
		srv.handleInstallSH(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `DEFAULT_DISCLOUD_BASE="https://api.example.com"`) {
			t.Fatalf("missing baked base:\n%s", body)
		}
		if !strings.Contains(body, `DEFAULT_DISCLOUD_ORIGIN="https://app.example.com"`) {
			t.Fatalf("missing baked origin:\n%s", body)
		}
		if strings.Contains(body, "{{DISCLOUD_") {
			t.Fatal("placeholders left unsubstituted")
		}
	})

	t.Run("ps1", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/install.ps1", nil)
		rec := httptest.NewRecorder()
		srv.handleInstallPS1(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `$DefaultBase = 'https://api.example.com'`) {
			t.Fatalf("missing baked base:\n%s", body)
		}
		if !strings.Contains(body, `$DefaultOrigin = 'https://app.example.com'`) {
			t.Fatalf("missing baked origin:\n%s", body)
		}
		if strings.Contains(body, "{{DISCLOUD_") {
			t.Fatal("placeholders left unsubstituted")
		}
	})
}

func TestInstallScriptUsesRequestHostWhenNoAPIURL(t *testing.T) {
	srv := &Server{webOrigin: "http://localhost:3000"}
	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	req.Host = "api.local:8080"
	rec := httptest.NewRecorder()
	srv.handleInstallSH(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `DEFAULT_DISCLOUD_BASE="http://api.local:8080"`) {
		t.Fatalf("want host-derived base, got:\n%s", body)
	}
}
