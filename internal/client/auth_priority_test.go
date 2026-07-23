package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthPriorityJarOverToken(t *testing.T) {
	var sawAuth, sawCookie, sawOrigin string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		if c, err := r.Cookie("discloud_session"); err == nil {
			sawCookie = c.Value
		}
		sawOrigin = r.Header.Get("Origin")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(ts.Close)

	dir := t.TempDir()
	cookiePath := filepath.Join(dir, "cookies")
	jarData, _ := json.Marshal(cookieFile{Cookies: []cookieEntry{
		{Name: "discloud_session", Value: "sess-from-jar", Path: "/"},
	}})
	if err := os.WriteFile(cookiePath, jarData, 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := New(Config{
		BaseURL:    ts.URL,
		Origin:     "http://localhost:3000",
		CookiePath: cookiePath,
		Token:      "dc_should_not_be_sent",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Do(http.MethodPost, "/api/auth/me", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if sawAuth != "" {
		t.Fatalf("Authorization = %q, want empty when jar has session", sawAuth)
	}
	if sawCookie != "sess-from-jar" {
		t.Fatalf("cookie = %q, want sess-from-jar", sawCookie)
	}
	if sawOrigin != "http://localhost:3000" {
		t.Fatalf("Origin = %q, want CSRF origin", sawOrigin)
	}
}

func TestAuthFallsBackToTokenWithoutJar(t *testing.T) {
	var sawAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(ts.Close)

	c, err := New(Config{
		BaseURL:    ts.URL,
		Origin:     "http://localhost:3000",
		CookiePath: filepath.Join(t.TempDir(), "cookies"),
		Token:      "dc_pat_token",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Do(http.MethodGet, "/api/auth/me", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if sawAuth != "Bearer dc_pat_token" {
		t.Fatalf("Authorization = %q, want Bearer dc_pat_token", sawAuth)
	}
}

func TestEmptyCookiePathSkipsJar(t *testing.T) {
	var sawAuth, sawCookie string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		if c, err := r.Cookie("discloud_session"); err == nil {
			sawCookie = c.Value
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(ts.Close)

	// Even if a default jar would exist, empty CookiePath must not load it.
	c, err := New(Config{
		BaseURL:    ts.URL,
		Origin:     "http://localhost:3000",
		CookiePath: "",
		Token:      "dc_validate_only",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Do(http.MethodGet, "/api/auth/me", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if sawAuth != "Bearer dc_validate_only" {
		t.Fatalf("Authorization = %q", sawAuth)
	}
	if sawCookie != "" {
		t.Fatalf("cookie = %q, want none", sawCookie)
	}
}

func TestLoginWithConfiguredTokenPersistsSessionCookie(t *testing.T) {
	var n int
	var firstAuth, firstOrigin string
	var secondAuth, secondCookie string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			firstAuth = r.Header.Get("Authorization")
			firstOrigin = r.Header.Get("Origin")
			http.SetCookie(w, &http.Cookie{Name: "discloud_session", Value: "new-sess", Path: "/"})
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"1","username":"admin","role":"admin"}`))
			return
		}
		secondAuth = r.Header.Get("Authorization")
		if c, err := r.Cookie("discloud_session"); err == nil {
			secondCookie = c.Value
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(ts.Close)

	dir := t.TempDir()
	c, err := New(Config{
		BaseURL:    ts.URL,
		Origin:     "http://localhost:3000",
		CookiePath: filepath.Join(dir, "cookies"),
		Token:      "dc_pat_without_admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SignIn("admin", "x"); err != nil {
		t.Fatal(err)
	}
	if firstAuth != "" {
		t.Fatalf("login Authorization = %q, want empty (cookie session path)", firstAuth)
	}
	if firstOrigin != "http://localhost:3000" {
		t.Fatalf("login Origin = %q", firstOrigin)
	}

	res, err := c.Do(http.MethodGet, "/api/admin/overview", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if secondAuth != "" {
		t.Fatalf("2nd Authorization = %q, want empty (jar should win after login)", secondAuth)
	}
	if secondCookie != "new-sess" {
		t.Fatalf("2nd cookie = %q, want new-sess", secondCookie)
	}
}

func TestPATWithAdminScopeUsedWhenNoJar(t *testing.T) {
	var sawAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(ts.Close)

	c, err := New(Config{
		BaseURL:    ts.URL,
		Origin:     "http://localhost:3000",
		CookiePath: filepath.Join(t.TempDir(), "cookies"),
		Token:      "dc_pat_with_admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Do(http.MethodGet, "/api/admin/overview", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if sawAuth != "Bearer dc_pat_with_admin" {
		t.Fatalf("Authorization = %q, want Bearer when jar empty", sawAuth)
	}
}
