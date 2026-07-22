package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/store"
)

const testWebOrigin = "http://localhost:3000"

type authEnv struct {
	ts    *httptest.Server
	srv   *Server
	st    *memStore
	fake  *fakeDiscord
	clock *atomic.Int64 // unix nano; 0 → time.Now
}

func (e *authEnv) now() time.Time {
	if n := e.clock.Load(); n != 0 {
		return time.Unix(0, n).UTC()
	}
	return time.Now().UTC()
}

func (e *authEnv) setNow(t time.Time) { e.clock.Store(t.UnixNano()) }

func newAuthEnv(t *testing.T) *authEnv {
	t.Helper()
	fake, discordTS := newFakeDiscord(t)
	dc := discord.New("test-token", "test-channel")
	dc.BaseURL = discordTS.URL
	ca := &memCache{urls: map[string]string{}}
	st := &memStore{files: map[string]store.File{}, chunks: map[string]store.Chunk{}}
	clock := &atomic.Int64{}
	opts := testOpts("")
	opts.Now = func() time.Time {
		if n := clock.Load(); n != 0 {
			return time.Unix(0, n).UTC()
		}
		return time.Now().UTC()
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(log, st, ca, dc, opts)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &authEnv{ts: ts, srv: srv, st: st, fake: fake, clock: clock}
}

func (e *authEnv) do(t *testing.T, method, path, cookie, body, contentType string) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if cookie != "" {
		req.Header.Set("Cookie", sessionCookieName+"="+cookie)
		req.Header.Set("Origin", testWebOrigin)
	} else if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		// Browser credentialed calls always send Origin; anonymous mutating may omit it.
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (e *authEnv) doJSON(t *testing.T, method, path, cookie string, payload any) *http.Response {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return e.do(t, method, path, cookie, string(b), "application/json")
}

func cookieFrom(resp *http.Response) string {
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return c.Value
		}
	}
	return ""
}

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (e *authEnv) signup(t *testing.T, username, password string) (cookie string, user map[string]any) {
	t.Helper()
	resp := e.doJSON(t, http.MethodPost, "/api/auth/signup", "", map[string]string{
		"username": username, "password": password,
	})
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("signup status = %d: %s", resp.StatusCode, body)
	}
	cookie = cookieFrom(resp)
	user = decodeJSON[map[string]any](t, resp)
	if cookie == "" {
		t.Fatal("missing session cookie")
	}
	return cookie, user
}

func (e *authEnv) upload(t *testing.T, cookie, name, data string) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.ts.URL+"/api/upload?fileName="+name,
		strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if cookie != "" {
		req.Header.Set("Cookie", sessionCookieName+"="+cookie)
		req.Header.Set("Origin", testWebOrigin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("upload status = %d: %s", resp.StatusCode, body)
	}
	return decodeJSON[map[string]any](t, resp)
}

func TestAuthSignupFirstAdminAndSession(t *testing.T) {
	e := newAuthEnv(t)
	c1, u1 := e.signup(t, "  Admin_User  ", "password1")
	if u1["role"] != store.RoleAdmin {
		t.Fatalf("first user role = %v, want admin", u1["role"])
	}
	if u1["username"] != "admin_user" {
		t.Fatalf("username not normalized: %v", u1["username"])
	}

	me := e.do(t, http.MethodGet, "/api/auth/me", c1, "", "")
	got := decodeJSON[map[string]any](t, me)
	if got["id"] != u1["id"] {
		t.Fatalf("me = %+v", got)
	}

	c2, u2 := e.signup(t, "user2", "password2")
	if u2["role"] != store.RoleUser {
		t.Fatalf("second user role = %v, want user", u2["role"])
	}

	dup := e.doJSON(t, http.MethodPost, "/api/auth/signup", "", map[string]string{
		"username": "USER2", "password": "password3",
	})
	if dup.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate username = %d, want 409", dup.StatusCode)
	}
	dup.Body.Close()

	bad := e.doJSON(t, http.MethodPost, "/api/auth/signin", "", map[string]string{
		"username": "user2", "password": "wrong-password",
	})
	if bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad signin = %d", bad.StatusCode)
	}
	bad.Body.Close()

	ok := e.doJSON(t, http.MethodPost, "/api/auth/signin", "", map[string]string{
		"username": "user2", "password": "password2",
	})
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("signin = %d", ok.StatusCode)
	}
	c2b := cookieFrom(ok)
	ok.Body.Close()
	if c2b == "" {
		t.Fatal("signin missing cookie")
	}

	out := e.do(t, http.MethodPost, "/api/auth/signout", c2, "", "")
	if out.StatusCode != http.StatusNoContent {
		t.Fatalf("signout = %d", out.StatusCode)
	}
	out.Body.Close()
	me2 := e.do(t, http.MethodGet, "/api/auth/me", c2, "", "")
	if me2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after signout me = %d", me2.StatusCode)
	}
	me2.Body.Close()
	_ = c2b
}

func TestAuthChangePassword(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "changer", "password1")

	wrong := e.doJSON(t, http.MethodPost, "/api/auth/password", cookie, map[string]string{
		"currentPassword": "wrong-password", "newPassword": "password2",
	})
	if wrong.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong current = %d", wrong.StatusCode)
	}
	wrong.Body.Close()

	short := e.doJSON(t, http.MethodPost, "/api/auth/password", cookie, map[string]string{
		"currentPassword": "password1", "newPassword": "short",
	})
	if short.StatusCode != http.StatusBadRequest {
		t.Fatalf("short new = %d", short.StatusCode)
	}
	short.Body.Close()

	other := e.doJSON(t, http.MethodPost, "/api/auth/signin", "", map[string]string{
		"username": "changer", "password": "password1",
	})
	if other.StatusCode != http.StatusOK {
		t.Fatalf("second signin = %d", other.StatusCode)
	}
	otherCookie := cookieFrom(other)
	other.Body.Close()
	if otherCookie == "" || otherCookie == cookie {
		t.Fatal("expected distinct second-device cookie")
	}

	before := e.do(t, http.MethodGet, "/api/auth/me", cookie, "", "")
	beforeBody := decodeJSON[map[string]any](t, before)
	prevChanged, _ := beforeBody["passwordChangedAt"].(string)

	ok := e.doJSON(t, http.MethodPost, "/api/auth/password", cookie, map[string]string{
		"currentPassword": "password1", "newPassword": "password2",
	})
	if ok.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(ok.Body)
		ok.Body.Close()
		t.Fatalf("change password = %d: %s", ok.StatusCode, body)
	}
	newCookie := cookieFrom(ok)
	ok.Body.Close()
	if newCookie == "" {
		t.Fatal("password change must re-issue session cookie")
	}

	stale := e.do(t, http.MethodGet, "/api/auth/me", cookie, "", "")
	if stale.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old cookie still valid = %d", stale.StatusCode)
	}
	stale.Body.Close()
	staleOther := e.do(t, http.MethodGet, "/api/auth/me", otherCookie, "", "")
	if staleOther.StatusCode != http.StatusUnauthorized {
		t.Fatalf("other-device cookie still valid = %d", staleOther.StatusCode)
	}
	staleOther.Body.Close()

	me := e.do(t, http.MethodGet, "/api/auth/me", newCookie, "", "")
	meBody := decodeJSON[map[string]any](t, me)
	if meBody["passwordChangedAt"] == prevChanged {
		t.Fatalf("passwordChangedAt not bumped: %v", meBody["passwordChangedAt"])
	}

	old := e.doJSON(t, http.MethodPost, "/api/auth/signin", "", map[string]string{
		"username": "changer", "password": "password1",
	})
	if old.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password still works = %d", old.StatusCode)
	}
	old.Body.Close()

	neu := e.doJSON(t, http.MethodPost, "/api/auth/signin", "", map[string]string{
		"username": "changer", "password": "password2",
	})
	if neu.StatusCode != http.StatusOK {
		t.Fatalf("new password signin = %d", neu.StatusCode)
	}
	neu.Body.Close()
}

func TestAccountDashboardMe(t *testing.T) {
	e := newAuthEnv(t)
	req, err := http.NewRequest(http.MethodPost, e.ts.URL+"/api/auth/signup", strings.NewReader(
		`{"username":"dashuser","password":"password1"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0")
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("signup = %d: %s", resp.StatusCode, body)
	}
	cookie := cookieFrom(resp)
	resp.Body.Close()
	if cookie == "" {
		t.Fatal("missing cookie")
	}

	up := e.upload(t, cookie, "a.txt", "hello")
	fileID := up["fileId"].(string)
	vis := e.doJSON(t, http.MethodPatch, "/api/files/"+fileID+"/visibility", cookie, map[string]string{
		"visibility": store.VisibilityPrivate,
	})
	if vis.StatusCode != http.StatusOK {
		t.Fatalf("visibility = %d", vis.StatusCode)
	}
	vis.Body.Close()
	_ = e.upload(t, cookie, "b.txt", "world!!")

	me := e.do(t, http.MethodGet, "/api/auth/me", cookie, "", "")
	if me.StatusCode != http.StatusOK {
		t.Fatalf("me = %d", me.StatusCode)
	}
	body := decodeJSON[map[string]any](t, me)
	if body["username"] != "dashuser" {
		t.Fatalf("username = %v", body["username"])
	}
	stats, _ := body["stats"].(map[string]any)
	if int(stats["fileCount"].(float64)) != 2 {
		t.Fatalf("fileCount = %v", stats["fileCount"])
	}
	if int(stats["privateCount"].(float64)) != 1 || int(stats["publicCount"].(float64)) != 1 {
		t.Fatalf("visibility counts = %+v", stats)
	}
	if int64(stats["totalBytes"].(float64)) != int64(len("hello")+len("world!!")) {
		t.Fatalf("totalBytes = %v", stats["totalBytes"])
	}
	sess, _ := body["session"].(map[string]any)
	if sess["ip"] == "" || sess["userAgent"] == "" {
		t.Fatalf("session meta = %+v", sess)
	}
	if sess["createdAt"] == nil || sess["lastSeenAt"] == nil || sess["expiresAt"] == nil {
		t.Fatalf("session times = %+v", sess)
	}
	ret, _ := body["retention"].(map[string]any)
	if int(ret["authenticatedDays"].(float64)) != 30 || int(ret["anonymousDays"].(float64)) != 7 {
		t.Fatalf("retention = %+v", ret)
	}
}

func TestAuthConcurrentFirstAdminMemStore(t *testing.T) {
	st := &memStore{}
	const n = 32
	results := make([]store.User, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = st.CreateUser(context.Background(),
				fmt.Sprintf("id-%d", i),
				fmt.Sprintf("u%d", i),
				"hash")
		}(i)
	}
	wg.Wait()
	admins := 0
	for i, u := range results {
		if errs[i] != nil {
			t.Fatalf("CreateUser %d: %v", i, errs[i])
		}
		if u.Role == store.RoleAdmin {
			admins++
		}
	}
	if admins != 1 {
		t.Fatalf("admin count = %d, want 1", admins)
	}
}

func parseTime(t *testing.T, v any) time.Time {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("expected time string, got %T %v", v, v)
	}
	tm, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		tm, err = time.Parse(time.RFC3339, s)
	}
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return tm.UTC()
}

func TestUploadRetentionOwnership(t *testing.T) {
	e := newAuthEnv(t)
	fixed := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	e.setNow(fixed)

	anon := e.upload(t, "", "anon.txt", "hello anon")
	expAnon := parseTime(t, anon["expiresAt"])
	wantAnon := fixed.Add(anonymousRetention)
	if !expAnon.Equal(wantAnon) {
		t.Fatalf("anon expiresAt = %v, want %v", expAnon, wantAnon)
	}
	if anon["visibility"] != store.VisibilityPublic {
		t.Fatalf("anon visibility = %v", anon["visibility"])
	}
	if anon["ownedByCurrentUser"] != false {
		t.Fatalf("anon ownedByCurrentUser = %v", anon["ownedByCurrentUser"])
	}

	cookie, _ := e.signup(t, "owner", "password1")
	owned := e.upload(t, cookie, "owned.txt", "hello owner")
	expOwn := parseTime(t, owned["expiresAt"])
	wantOwn := fixed.Add(authenticatedRetention)
	if !expOwn.Equal(wantOwn) {
		t.Fatalf("owned expiresAt = %v, want %v", expOwn, wantOwn)
	}
	if owned["ownedByCurrentUser"] != true {
		t.Fatalf("owned ownedByCurrentUser = %v", owned["ownedByCurrentUser"])
	}
}

func TestDefaultVisibilityPreference(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "prefs", "password1")

	me := e.do(t, http.MethodGet, "/api/auth/me", cookie, "", "")
	if me.StatusCode != http.StatusOK {
		t.Fatalf("me = %d", me.StatusCode)
	}
	meBody := decodeJSON[map[string]any](t, me)
	prefs := meBody["preferences"].(map[string]any)
	if prefs["defaultVisibility"] != store.VisibilityPublic {
		t.Fatalf("default visibility = %v", prefs["defaultVisibility"])
	}

	patch := e.doJSON(t, http.MethodPatch, "/api/auth/preferences", cookie, map[string]string{
		"defaultVisibility": store.VisibilityPrivate,
	})
	if patch.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(patch.Body)
		patch.Body.Close()
		t.Fatalf("preferences = %d: %s", patch.StatusCode, body)
	}
	patchBody := decodeJSON[map[string]any](t, patch)
	if patchBody["preferences"].(map[string]any)["defaultVisibility"] != store.VisibilityPrivate {
		t.Fatalf("patch body = %+v", patchBody)
	}

	up := e.upload(t, cookie, "secret.txt", "private by default")
	if up["visibility"] != store.VisibilityPrivate {
		t.Fatalf("upload visibility = %v", up["visibility"])
	}
	raw, _ := up["accessToken"].(string)
	if raw == "" {
		t.Fatal("expected accessToken on private default upload")
	}
	fileID := up["fileId"].(string)

	// Stranger without token → 404
	denied := e.do(t, http.MethodGet, "/api/files/"+fileID, "", "", "")
	if denied.StatusCode != http.StatusNotFound {
		t.Fatalf("stranger status = %d", denied.StatusCode)
	}
	denied.Body.Close()

	// Token works
	ok := e.do(t, http.MethodGet, "/api/files/"+fileID+"?token="+raw, "", "", "")
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("token access = %d", ok.StatusCode)
	}
	ok.Body.Close()
}

func TestPublicPrivateAccessMatrix(t *testing.T) {
	e := newAuthEnv(t)
	ownerCookie, _ := e.signup(t, "owner", "password1")
	otherCookie, _ := e.signup(t, "other", "password2")
	up := e.upload(t, ownerCookie, "secret.txt", "top secret")
	fileID := up["fileId"].(string)

	// Public: anyone can read.
	pub := e.do(t, http.MethodGet, "/api/files/"+fileID, "", "", "")
	if pub.StatusCode != http.StatusOK {
		t.Fatalf("public get = %d", pub.StatusCode)
	}
	pub.Body.Close()

	vis := e.doJSON(t, http.MethodPatch, "/api/files/"+fileID+"/visibility", ownerCookie, map[string]string{
		"visibility": store.VisibilityPrivate,
	})
	if vis.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(vis.Body)
		vis.Body.Close()
		t.Fatalf("visibility = %d: %s", vis.StatusCode, body)
	}
	tokBody := decodeJSON[map[string]any](t, vis)
	token, _ := tokBody["accessToken"].(string)
	if token == "" {
		t.Fatal("missing accessToken on private switch")
	}

	// No token / wrong session → 404.
	deny := e.do(t, http.MethodGet, "/api/files/"+fileID, "", "", "")
	if deny.StatusCode != http.StatusNotFound {
		t.Fatalf("private no auth = %d, want 404", deny.StatusCode)
	}
	deny.Body.Close()
	deny2 := e.do(t, http.MethodGet, "/api/files/"+fileID, otherCookie, "", "")
	if deny2.StatusCode != http.StatusNotFound {
		t.Fatalf("private other user = %d, want 404", deny2.StatusCode)
	}
	deny2.Body.Close()

	// Owner session OK.
	own := e.do(t, http.MethodGet, "/api/files/"+fileID, ownerCookie, "", "")
	if own.StatusCode != http.StatusOK {
		t.Fatalf("owner get private = %d", own.StatusCode)
	}
	if rp := own.Header.Get("Referrer-Policy"); rp != "" {
		t.Fatalf("owner session should not force no-referrer, got %q", rp)
	}
	own.Body.Close()

	// Query token OK + Referrer-Policy.
	tok := e.do(t, http.MethodGet, "/api/files/"+fileID+"?token="+token, "", "", "")
	if tok.StatusCode != http.StatusOK {
		t.Fatalf("token query = %d", tok.StatusCode)
	}
	if tok.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q", tok.Header.Get("Referrer-Policy"))
	}
	tok.Body.Close()

	// Header token OK.
	req, _ := http.NewRequest(http.MethodGet, e.ts.URL+"/api/files/"+fileID, nil)
	req.Header.Set("X-File-Token", token)
	hdr, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if hdr.StatusCode != http.StatusOK {
		t.Fatalf("X-File-Token = %d", hdr.StatusCode)
	}
	hdr.Body.Close()

	// Download path same gate.
	dlDeny := e.do(t, http.MethodGet, "/f/"+fileID+"?json=1", "", "", "")
	if dlDeny.StatusCode != http.StatusNotFound {
		t.Fatalf("private download no token = %d", dlDeny.StatusCode)
	}
	dlDeny.Body.Close()
	dlOK := e.do(t, http.MethodGet, "/f/"+fileID+"?json=1&token="+token, "", "", "")
	if dlOK.StatusCode != http.StatusOK {
		t.Fatalf("private download token = %d", dlOK.StatusCode)
	}
	dlOK.Body.Close()

	// Token alone cannot manage.
	rot := e.doJSON(t, http.MethodPost, "/api/files/"+fileID+"/access-token/rotate", "", map[string]any{})
	if rot.StatusCode != http.StatusUnauthorized {
		t.Fatalf("rotate without session = %d", rot.StatusCode)
	}
	rot.Body.Close()
	// Even with Origin + fake cookie of nothing — use other user.
	rot2 := e.do(t, http.MethodPost, "/api/files/"+fileID+"/access-token/rotate", otherCookie, "", "")
	if rot2.StatusCode != http.StatusNotFound {
		t.Fatalf("rotate by non-owner = %d, want 404", rot2.StatusCode)
	}
	rot2.Body.Close()

	// Rotate invalidates old token.
	rot3 := e.doJSON(t, http.MethodPost, "/api/files/"+fileID+"/access-token/rotate", ownerCookie, map[string]any{})
	newTok := decodeJSON[map[string]any](t, rot3)
	if newTok["accessToken"] == "" || newTok["accessToken"] == token {
		t.Fatalf("rotate token = %v", newTok)
	}
	old := e.do(t, http.MethodGet, "/api/files/"+fileID+"?token="+token, "", "", "")
	if old.StatusCode != http.StatusNotFound {
		t.Fatalf("old token still works: %d", old.StatusCode)
	}
	old.Body.Close()

	// Public switch clears token.
	pub2 := e.doJSON(t, http.MethodPatch, "/api/files/"+fileID+"/visibility", ownerCookie, map[string]string{
		"visibility": store.VisibilityPublic,
	})
	if pub2.StatusCode != http.StatusOK {
		t.Fatalf("public switch = %d", pub2.StatusCode)
	}
	pub2.Body.Close()
	again := e.do(t, http.MethodGet, "/api/files/"+fileID, "", "", "")
	if again.StatusCode != http.StatusOK {
		t.Fatalf("after public = %d", again.StatusCode)
	}
	again.Body.Close()

	// Anonymous upload cannot go private.
	anonUp := e.upload(t, "", "anon2.txt", "x")
	anonID := anonUp["fileId"].(string)
	// Need a session that can "manage" — admin can, but anonymous owner is nil → 403.
	adminCookie, _ := e.signup(t, "admin2", "password1") // third user = user role
	// First user is still admin (owner)
	forbid := e.doJSON(t, http.MethodPatch, "/api/files/"+anonID+"/visibility", ownerCookie, map[string]string{
		"visibility": store.VisibilityPrivate,
	})
	if forbid.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(forbid.Body)
		forbid.Body.Close()
		t.Fatalf("anon private = %d: %s", forbid.StatusCode, body)
	}
	forbid.Body.Close()
	_ = adminCookie
}

func TestListAndDeleteNoDiscord(t *testing.T) {
	e := newAuthEnv(t)
	c1, _ := e.signup(t, "user_a", "password1")
	c2, _ := e.signup(t, "user_b", "password2")
	up1 := e.upload(t, c1, "a.txt", "aaa")
	up2 := e.upload(t, c2, "b.txt", "bbb")
	id1 := up1["fileId"].(string)
	id2 := up2["fileId"].(string)
	uploadsBefore := e.fake.uploads

	list := e.do(t, http.MethodGet, "/api/files", c1, "", "")
	body := decodeJSON[map[string]any](t, list)
	files, _ := body["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("owner list len = %d, want 1", len(files))
	}
	f0 := files[0].(map[string]any)
	if f0["fileId"] != id1 {
		t.Fatalf("listed %v, want %s", f0["fileId"], id1)
	}

	// Non-owner delete → 404; Discord untouched.
	del := e.do(t, http.MethodDelete, "/api/files/"+id1, c2, "", "")
	if del.StatusCode != http.StatusNotFound {
		t.Fatalf("non-owner delete = %d", del.StatusCode)
	}
	del.Body.Close()

	delOK := e.do(t, http.MethodDelete, "/api/files/"+id1, c1, "", "")
	if delOK.StatusCode != http.StatusNoContent {
		t.Fatalf("owner delete = %d", delOK.StatusCode)
	}
	delOK.Body.Close()
	if e.fake.uploads != uploadsBefore {
		t.Fatalf("discord uploads changed on delete: %d → %d", uploadsBefore, e.fake.uploads)
	}

	gone := e.do(t, http.MethodGet, "/api/files/"+id1, c1, "", "")
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("after delete get = %d", gone.StatusCode)
	}
	gone.Body.Close()

	// Token cannot delete.
	up3 := e.upload(t, c1, "c.txt", "ccc")
	id3 := up3["fileId"].(string)
	vis := e.doJSON(t, http.MethodPatch, "/api/files/"+id3+"/visibility", c1, map[string]string{
		"visibility": store.VisibilityPrivate,
	})
	tok := decodeJSON[map[string]any](t, vis)["accessToken"].(string)
	req, _ := http.NewRequest(http.MethodDelete, e.ts.URL+"/api/files/"+id3+"?token="+tok, nil)
	req.Header.Set("Origin", testWebOrigin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token delete = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
	_ = id2
}

func TestDownloadExtension(t *testing.T) {
	e := newAuthEnv(t)
	fixed := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	e.setNow(fixed)
	up := e.upload(t, "", "tiny.txt", "extend-me")
	id := up["fileId"].(string)

	f, err := e.st.GetFile(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	baseExp := f.ExpiresAt

	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// Inline GET must NOT extend.
	r1, err := noFollow.Get(e.ts.URL + "/f/" + id)
	if err != nil {
		t.Fatal(err)
	}
	r1.Body.Close()
	f, _ = e.st.GetFile(context.Background(), id)
	if !f.ExpiresAt.Equal(baseExp) {
		t.Fatalf("inline extended: %v → %v", baseExp, f.ExpiresAt)
	}

	// HEAD download must NOT extend.
	req, _ := http.NewRequest(http.MethodHead, e.ts.URL+"/f/"+id+"?download=1", nil)
	r2, err := noFollow.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	f, _ = e.st.GetFile(context.Background(), id)
	if !f.ExpiresAt.Equal(baseExp) {
		t.Fatalf("HEAD extended: %v → %v", baseExp, f.ExpiresAt)
	}

	// Range download must NOT extend.
	req, _ = http.NewRequest(http.MethodGet, e.ts.URL+"/f/"+id+"?download=1", nil)
	req.Header.Set("Range", "bytes=0-1")
	r3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, r3.Body)
	r3.Body.Close()
	f, _ = e.st.GetFile(context.Background(), id)
	if !f.ExpiresAt.Equal(baseExp) {
		t.Fatalf("Range extended: %v → %v", baseExp, f.ExpiresAt)
	}

	// Full ?download=1 extends by downloadExtension, capped at now+maxRetentionFromNow.
	r4, err := noFollow.Get(e.ts.URL + "/f/" + id + "?download=1")
	if err != nil {
		t.Fatal(err)
	}
	r4.Body.Close()
	f, _ = e.st.GetFile(context.Background(), id)
	want := baseExp.Add(downloadExtension)
	capAt := fixed.Add(maxRetentionFromNow)
	if want.After(capAt) {
		want = capAt
	}
	if !f.ExpiresAt.Equal(want) {
		t.Fatalf("after download expiresAt = %v, want %v", f.ExpiresAt, want)
	}
}

func TestCleanupExpiredAndCORS(t *testing.T) {
	e := newAuthEnv(t)
	fixed := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	e.setNow(fixed)
	up := e.upload(t, "", "gone.txt", "bye")
	id := up["fileId"].(string)
	uploadsBefore := e.fake.uploads

	// Still valid.
	ok := e.do(t, http.MethodGet, "/api/files/"+id, "", "", "")
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("before expire = %d", ok.StatusCode)
	}
	ok.Body.Close()

	// Advance past expiry and cleanup.
	e.setNow(fixed.Add(anonymousRetention + time.Hour))
	n, err := e.st.DeleteExpiredFiles(context.Background(), e.now(), cleanupBatchSize)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("cleanup deleted = %d, want 1", n)
	}
	if e.fake.uploads != uploadsBefore {
		t.Fatal("cleanup must not touch Discord")
	}
	gone := e.do(t, http.MethodGet, "/api/files/"+id, "", "", "")
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("after cleanup = %d", gone.StatusCode)
	}
	gone.Body.Close()

	// CORS: matching origin gets credentials headers.
	req, _ := http.NewRequest(http.MethodOptions, e.ts.URL+"/api/files", nil)
	req.Header.Set("Origin", testWebOrigin)
	opt, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	opt.Body.Close()
	if opt.Header.Get("Access-Control-Allow-Origin") != testWebOrigin {
		t.Fatalf("ACAO = %q", opt.Header.Get("Access-Control-Allow-Origin"))
	}
	if opt.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("missing Allow-Credentials")
	}

	// Wrong Origin on mutating + cookie → 403.
	cookie, _ := e.signup(t, "corsuser", "password1")
	bad, err := http.NewRequest(http.MethodPost, e.ts.URL+"/api/auth/signout", nil)
	if err != nil {
		t.Fatal(err)
	}
	bad.Header.Set("Cookie", sessionCookieName+"="+cookie)
	bad.Header.Set("Origin", "http://evil.example")
	resp, err := http.DefaultClient.Do(bad)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("evil origin = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestExpiredFileUniform404(t *testing.T) {
	e := newAuthEnv(t)
	fixed := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	e.setNow(fixed)
	up := e.upload(t, "", "exp.txt", "data")
	id := up["fileId"].(string)
	e.setNow(fixed.Add(anonymousRetention + time.Second))
	// Authz layer treats expired as not found (row still present until cleanup).
	resp := e.do(t, http.MethodGet, "/api/files/"+id, "", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expired get = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCSRFOriginOnMutatingWithoutCookie(t *testing.T) {
	e := newAuthEnv(t)
	// Mutating with mismatched Origin (no cookie) → 403.
	req, _ := http.NewRequest(http.MethodPost, e.ts.URL+"/api/upload?fileName=x.txt", bytes.NewReader([]byte("hi")))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Origin", "http://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("mismatched origin upload = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// Signed-in top-level navigation to a public file: cookie present, no Origin.
func TestPublicPreviewNoOriginWithCookie(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "preview", "password1")
	up := e.upload(t, cookie, "pub.txt", "hello public")
	id := up["fileId"].(string)

	noFollow := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(http.MethodGet, e.ts.URL+"/f/"+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", sessionCookieName+"="+cookie)
	// Intentionally no Origin — mirrors address-bar / <a href> navigation.
	resp, err := noFollow.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		t.Fatalf("public GET /f/{id} with cookie and no Origin must not be 403")
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302 CDN redirect", resp.StatusCode)
	}
}

func TestAdminCrossTenantPrivateAccess(t *testing.T) {
	e := newAuthEnv(t)
	adminCookie, _ := e.signup(t, "admin", "password1")
	ownerCookie, _ := e.signup(t, "owner", "password1")
	strangerCookie, _ := e.signup(t, "stranger", "password1")

	up := e.upload(t, ownerCookie, "secret.txt", "private-bytes")
	id := up["fileId"].(string)
	vis := e.doJSON(t, http.MethodPatch, "/api/files/"+id+"/visibility", ownerCookie, map[string]string{
		"visibility": "private",
	})
	if vis.StatusCode != http.StatusOK {
		t.Fatalf("make private = %d", vis.StatusCode)
	}
	vis.Body.Close()

	adminGet := e.do(t, http.MethodGet, "/api/files/"+id, adminCookie, "", "")
	if adminGet.StatusCode != http.StatusOK {
		t.Fatalf("admin get file = %d", adminGet.StatusCode)
	}
	adminGet.Body.Close()

	adminJSON := e.do(t, http.MethodGet, "/f/"+id+"?json=1", adminCookie, "", "")
	if adminJSON.StatusCode != http.StatusOK {
		t.Fatalf("admin /f?json=1 = %d", adminJSON.StatusCode)
	}
	adminJSON.Body.Close()

	strangerGet := e.do(t, http.MethodGet, "/api/files/"+id, strangerCookie, "", "")
	if strangerGet.StatusCode != http.StatusNotFound {
		t.Fatalf("stranger get = %d, want 404", strangerGet.StatusCode)
	}
	strangerGet.Body.Close()

	del := e.do(t, http.MethodDelete, "/api/files/"+id, adminCookie, "", "")
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("admin delete = %d", del.StatusCode)
	}
	del.Body.Close()
}

func TestPrivateInspectAuthz(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "owner", "password1")
	up := e.upload(t, cookie, "secret.txt", "private-bytes")
	id := up["fileId"].(string)
	vis := e.doJSON(t, http.MethodPatch, "/api/files/"+id+"/visibility", cookie, map[string]string{
		"visibility": "private",
	})
	body := decodeJSON[map[string]any](t, vis)
	token, _ := body["accessToken"].(string)
	if token == "" {
		t.Fatal("missing accessToken")
	}

	denied := e.do(t, http.MethodGet, "/api/files/"+id+"/inspect", "", "", "")
	if denied.StatusCode != http.StatusNotFound {
		t.Fatalf("anon inspect = %d, want 404", denied.StatusCode)
	}
	denied.Body.Close()

	owner := e.do(t, http.MethodGet, "/api/files/"+id+"/inspect", cookie, "", "")
	if owner.StatusCode != http.StatusOK {
		t.Fatalf("owner inspect = %d", owner.StatusCode)
	}
	owner.Body.Close()

	viaToken := e.do(t, http.MethodGet, "/api/files/"+id+"/inspect?token="+token, "", "", "")
	if viaToken.StatusCode != http.StatusOK {
		t.Fatalf("token inspect = %d", viaToken.StatusCode)
	}
	viaToken.Body.Close()
}

func TestChunkedCompleteWithSessionPrivateDefault(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "uploader", "password1")
	pref := e.doJSON(t, http.MethodPatch, "/api/auth/preferences", cookie, map[string]string{
		"defaultVisibility": "private",
	})
	if pref.StatusCode != http.StatusOK {
		t.Fatalf("preferences = %d", pref.StatusCode)
	}
	pref.Body.Close()

	data := []byte("chunked-owned-private")
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	req, err := http.NewRequest(http.MethodPost, e.ts.URL+"/api/chunks", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Cookie", sessionCookieName+"="+cookie)
	req.Header.Set("Origin", testWebOrigin)
	chunkResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if chunkResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(chunkResp.Body)
		chunkResp.Body.Close()
		t.Fatalf("chunk upload = %d: %s", chunkResp.StatusCode, body)
	}
	chunkResp.Body.Close()

	completeBody, _ := json.Marshal(map[string]any{
		"fileName": "owned.bin", "chunkHashes": []string{hash},
	})
	creq, err := http.NewRequest(http.MethodPost, e.ts.URL+"/api/upload/complete", bytes.NewReader(completeBody))
	if err != nil {
		t.Fatal(err)
	}
	creq.Header.Set("Content-Type", "application/json")
	creq.Header.Set("Cookie", sessionCookieName+"="+cookie)
	creq.Header.Set("Origin", testWebOrigin)
	cresp, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	if cresp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(cresp.Body)
		cresp.Body.Close()
		t.Fatalf("complete = %d: %s", cresp.StatusCode, body)
	}
	out := decodeJSON[map[string]any](t, cresp)
	if out["visibility"] != "private" {
		t.Fatalf("visibility = %v, want private", out["visibility"])
	}
	if out["accessToken"] == nil || out["accessToken"] == "" {
		t.Fatal("expected accessToken for private default")
	}
	if out["ownedByCurrentUser"] != true {
		t.Fatalf("ownedByCurrentUser = %v", out["ownedByCurrentUser"])
	}
}
