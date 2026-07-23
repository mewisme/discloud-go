package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/store"
)

func newAbuseEnv(t *testing.T, opts Options) (*httptest.Server, *Server, *memStore) {
	t.Helper()
	fake, discordTS := newFakeDiscord(t)
	dc := discord.New("test-token", "test-channel")
	dc.BaseURL = discordTS.URL
	ca := &memCache{urls: map[string]string{}}
	st := &memStore{files: map[string]store.File{}, chunks: map[string]store.Chunk{}}
	base := testOpts("")
	base.RateLimitUploadPerMin = opts.RateLimitUploadPerMin
	base.RateLimitDownloadPerMin = opts.RateLimitDownloadPerMin
	base.MaxUserBytes = opts.MaxUserBytes
	base.MaxAnonUploadsPerDay = opts.MaxAnonUploadsPerDay
	base.MaxAnonBytesPerDay = opts.MaxAnonBytesPerDay
	base.MaxRawUploadBytes = opts.MaxRawUploadBytes
	base.CaptchaSecret = opts.CaptchaSecret
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(log, st, ca, dc, base)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	_ = fake
	return ts, srv, st
}

func abuseSignup(t *testing.T, ts *httptest.Server, username string) (cookie string, userID string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": "password1"})
	resp, err := http.Post(ts.URL+"/api/auth/signup", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("signup = %d: %s", resp.StatusCode, b)
	}
	var u struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		t.Fatal(err)
	}
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return c.Value, u.ID
		}
	}
	t.Fatal("missing session cookie")
	return "", ""
}

func TestAnonUploadSessionRateLimit(t *testing.T) {
	ts, _, _ := newAbuseEnv(t, Options{RateLimitUploadPerMin: 2})
	body, _ := json.Marshal(map[string]any{"fileName": "a.txt", "fileSize": 1})
	for i := 0; i < 2; i++ {
		resp, err := http.Post(ts.URL+"/api/uploads", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("create %d status = %d, want 200", i, resp.StatusCode)
		}
	}
	resp, err := http.Post(ts.URL+"/api/uploads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("flood status = %d, want 429: %s", resp.StatusCode, b)
	}
}

func TestAnonDailyUploadCap(t *testing.T) {
	ts, _, _ := newAbuseEnv(t, Options{MaxAnonUploadsPerDay: 1})
	body, _ := json.Marshal(map[string]any{"fileName": "a.txt", "fileSize": 1})
	resp, err := http.Post(ts.URL+"/api/uploads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first create = %d", resp.StatusCode)
	}
	resp, err = http.Post(ts.URL+"/api/uploads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("second create = %d, want 429: %s", resp.StatusCode, b)
	}
}

func TestUserQuotaBlocksCreate(t *testing.T) {
	ts, srv, st := newAbuseEnv(t, Options{MaxUserBytes: 100})
	cookie, userID := abuseSignup(t, ts, "quota1")
	now := srv.now().UTC()
	uid := userID
	if err := st.CreateFile(context.Background(), store.File{
		ID: newID(), Name: "big.bin", Size: 80, ChunkSize: chunkSize,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour), OwnerUserID: &uid,
		Visibility: store.VisibilityPublic, Status: store.FileStatusReady,
	}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{"fileName": "more.txt", "fileSize": 50})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/uploads", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", sessionCookieName+"="+cookie)
	req.Header.Set("Origin", "http://localhost:3000")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInsufficientStorage {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create = %d, want 507: %s", resp.StatusCode, b)
	}
}

func TestRawUploadMaxBytes(t *testing.T) {
	ts, _, _ := newAbuseEnv(t, Options{MaxRawUploadBytes: 8})
	resp, err := http.Post(ts.URL+"/api/upload?fileName=big.txt", "application/octet-stream",
		strings.NewReader("0123456789"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 413: %s", resp.StatusCode, b)
	}
}

func TestCaptchaRequiredForAnonCreate(t *testing.T) {
	ts, srv, _ := newAbuseEnv(t, Options{
		CaptchaSecret: "test-secret",
	})
	srv.captchaVerify = func(ctx context.Context, secret, token, ip string) error {
		if token != "ok-token" {
			return errQuotaExceeded
		}
		return nil
	}
	body, _ := json.Marshal(map[string]any{"fileName": "a.txt", "fileSize": 1})
	resp, err := http.Post(ts.URL+"/api/uploads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing captcha = %d, want 400", resp.StatusCode)
	}
	body, _ = json.Marshal(map[string]any{"fileName": "a.txt", "fileSize": 1, "captchaToken": "bad"})
	resp, err = http.Post(ts.URL+"/api/uploads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad captcha = %d, want 403", resp.StatusCode)
	}
	body, _ = json.Marshal(map[string]any{"fileName": "a.txt", "fileSize": 1, "captchaToken": "ok-token"})
	resp, err = http.Post(ts.URL+"/api/uploads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("good captcha = %d: %s", resp.StatusCode, b)
	}
}
