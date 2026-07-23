package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

func (e *authEnv) doBearer(t *testing.T, method, path, token, body, contentType string) *http.Response {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (e *authEnv) doBearerJSON(t *testing.T, method, path, token string, payload any) *http.Response {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return e.doBearer(t, method, path, token, string(b), "application/json")
}

func (e *authEnv) createPAT(t *testing.T, cookie string, name string, scopes []string, expiresAt *string) map[string]any {
	t.Helper()
	body := map[string]any{"name": name, "scopes": scopes}
	if expiresAt != nil {
		body["expiresAt"] = *expiresAt
	}
	resp := e.doJSON(t, http.MethodPost, "/api/auth/tokens", cookie, body)
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create token = %d: %s", resp.StatusCode, b)
	}
	return decodeJSON[map[string]any](t, resp)
}

func TestAPITokenBearerUploadAndScopes(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "patuser", "password1")

	created := e.createPAT(t, cookie, "ci", []string{store.ScopeUpload, store.ScopeRead}, nil)
	raw, _ := created["token"].(string)
	if !strings.HasPrefix(raw, "dc_") {
		t.Fatalf("token prefix = %q", raw)
	}

	// Bearer upload OK
	up := e.doBearer(t, http.MethodPost, "/api/upload?fileName=pat.txt", raw, "hello-pat", "application/octet-stream")
	if up.StatusCode != http.StatusOK && up.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(up.Body)
		up.Body.Close()
		t.Fatalf("bearer upload = %d: %s", up.StatusCode, b)
	}
	file := decodeJSON[map[string]any](t, up)
	fileID, _ := file["fileId"].(string)
	if fileID == "" {
		t.Fatal("expected fileId")
	}

	// List with read OK
	list := e.doBearer(t, http.MethodGet, "/api/files", raw, "", "")
	if list.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(list.Body)
		list.Body.Close()
		t.Fatalf("list = %d: %s", list.StatusCode, b)
	}
	list.Body.Close()

	// Manage without manage scope → 403
	vis := e.doBearerJSON(t, http.MethodPatch, "/api/files/"+fileID+"/visibility", raw, map[string]string{
		"visibility": "private",
	})
	if vis.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(vis.Body)
		vis.Body.Close()
		t.Fatalf("visibility without manage = %d: %s", vis.StatusCode, b)
	}
	vis.Body.Close()

	// Upload-only token cannot list
	uploadOnly := e.createPAT(t, cookie, "up", []string{store.ScopeUpload}, nil)
	upTok, _ := uploadOnly["token"].(string)
	badList := e.doBearer(t, http.MethodGet, "/api/files", upTok, "", "")
	if badList.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(badList.Body)
		badList.Body.Close()
		t.Fatalf("list without read = %d: %s", badList.StatusCode, b)
	}
	badList.Body.Close()

	// Cookie still works without scopes on manage
	okVis := e.doJSON(t, http.MethodPatch, "/api/files/"+fileID+"/visibility", cookie, map[string]string{
		"visibility": "public",
	})
	if okVis.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(okVis.Body)
		okVis.Body.Close()
		t.Fatalf("cookie visibility = %d: %s", okVis.StatusCode, b)
	}
	okVis.Body.Close()
}

func TestAPITokenRevokedAndExpired(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "patrev", "password1")

	created := e.createPAT(t, cookie, "soon", []string{store.ScopeUpload}, nil)
	raw, _ := created["token"].(string)
	id, _ := created["id"].(string)

	rev := e.do(t, http.MethodDelete, "/api/auth/tokens/"+id, cookie, "", "")
	if rev.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(rev.Body)
		rev.Body.Close()
		t.Fatalf("revoke = %d: %s", rev.StatusCode, b)
	}
	rev.Body.Close()

	up := e.doBearer(t, http.MethodPost, "/api/upload?fileName=x.txt", raw, "x", "application/octet-stream")
	if up.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(up.Body)
		up.Body.Close()
		t.Fatalf("revoked upload = %d: %s", up.StatusCode, b)
	}
	up.Body.Close()

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	e.setNow(now)
	exp := now.Add(time.Hour).Format(time.RFC3339)
	expiredTok := e.createPAT(t, cookie, "exp", []string{store.ScopeUpload}, &exp)
	rawExp, _ := expiredTok["token"].(string)
	e.setNow(now.Add(2 * time.Hour))

	up2 := e.doBearer(t, http.MethodPost, "/api/upload?fileName=y.txt", rawExp, "y", "application/octet-stream")
	if up2.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(up2.Body)
		up2.Body.Close()
		t.Fatalf("expired upload = %d: %s", up2.StatusCode, b)
	}
	up2.Body.Close()
}

func TestAPITokenCSRFBearerOnly(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "patcsrf", "password1")
	created := e.createPAT(t, cookie, "ci", []string{store.ScopeManage, store.ScopeUpload}, nil)
	raw, _ := created["token"].(string)

	// Bearer POST without Origin succeeds
	resp := e.doBearerJSON(t, http.MethodPost, "/api/auth/tokens", raw, map[string]any{
		"name":   "second",
		"scopes": []string{store.ScopeRead},
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("bearer create without Origin = %d: %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Cookie mutating without Origin fails CSRF
	req, _ := http.NewRequest(http.MethodPost, e.ts.URL+"/api/auth/tokens", strings.NewReader(`{"name":"x","scopes":["read"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", sessionCookieName+"="+cookie)
	bad, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(bad.Body)
		t.Fatalf("cookie without Origin = %d: %s", bad.StatusCode, b)
	}
}

func TestAPITokenWrongScopeOnUpload(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "patnoscope", "password1")
	created := e.createPAT(t, cookie, "read-only", []string{store.ScopeRead}, nil)
	raw, _ := created["token"].(string)

	up := e.doBearer(t, http.MethodPost, "/api/upload?fileName=no.txt", raw, "nope", "application/octet-stream")
	if up.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(up.Body)
		up.Body.Close()
		t.Fatalf("upload without upload scope = %d: %s", up.StatusCode, b)
	}
	up.Body.Close()
}
