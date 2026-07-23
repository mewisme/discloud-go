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

func TestSharePasswordGate(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "sharepw", "password1")

	up := e.do(t, http.MethodPost, "/api/upload?fileName=secret.txt", cookie, "payload-secret", "application/octet-stream")
	created := decodeJSON[map[string]any](t, up)
	fileID := created["fileId"].(string)

	resp := e.doJSON(t, http.MethodPatch, "/api/files/"+fileID+"/share", cookie, map[string]any{
		"password": "lock-me-in",
	})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("share patch = %d: %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Anonymous without password → 401 password_required
	get := e.do(t, http.MethodGet, "/api/files/"+fileID, "", "", "")
	if get.StatusCode != http.StatusUnauthorized {
		t.Fatalf("get without password = %d", get.StatusCode)
	}
	var errBody map[string]string
	_ = json.NewDecoder(get.Body).Decode(&errBody)
	get.Body.Close()
	if errBody["code"] != "password_required" {
		t.Fatalf("code = %q", errBody["code"])
	}

	// Wrong password header → password_invalid
	req, _ := http.NewRequest(http.MethodGet, e.ts.URL+"/api/files/"+fileID, nil)
	req.Header.Set(filePasswordHeader, "wrong-password")
	bad, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong pw = %d", bad.StatusCode)
	}
	_ = json.NewDecoder(bad.Body).Decode(&errBody)
	bad.Body.Close()
	if errBody["code"] != "password_invalid" {
		t.Fatalf("code = %q", errBody["code"])
	}

	// Unlock then access
	unlock := e.doJSON(t, http.MethodPost, "/api/files/"+fileID+"/unlock", "", map[string]string{
		"password": "lock-me-in",
	})
	if unlock.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(unlock.Body)
		unlock.Body.Close()
		t.Fatalf("unlock = %d: %s", unlock.StatusCode, b)
	}
	var unlockCookie string
	for _, c := range unlock.Cookies() {
		if strings.HasPrefix(c.Name, "discloud_unlock_") {
			unlockCookie = c.Name + "=" + c.Value
		}
	}
	unlock.Body.Close()
	if unlockCookie == "" {
		t.Fatal("expected unlock cookie")
	}

	req2, _ := http.NewRequest(http.MethodGet, e.ts.URL+"/api/files/"+fileID, nil)
	req2.Header.Set("Cookie", unlockCookie)
	ok, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(ok.Body)
		t.Fatalf("get after unlock = %d: %s", ok.StatusCode, b)
	}
}

func TestShareDownloadLimitAndViewMode(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "sharelim", "password1")

	up := e.do(t, http.MethodPost, "/api/upload?fileName=cap.txt", cookie, "cap-body", "application/octet-stream")
	created := decodeJSON[map[string]any](t, up)
	fileID := created["fileId"].(string)

	resp := e.doJSON(t, http.MethodPatch, "/api/files/"+fileID+"/share", cookie, map[string]any{
		"maxDownloads": 1,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share = %d", resp.StatusCode)
	}

	// First anonymous download succeeds
	dl1 := e.do(t, http.MethodGet, "/f/"+fileID+"?download=1", "", "", "")
	dl1.Body.Close()
	if dl1.StatusCode != http.StatusFound && dl1.StatusCode != http.StatusOK {
		t.Fatalf("first download = %d", dl1.StatusCode)
	}

	// Second hits 410
	dl2 := e.do(t, http.MethodGet, "/f/"+fileID+"?download=1", "", "", "")
	dl2.Body.Close()
	if dl2.StatusCode != http.StatusGone {
		t.Fatalf("second download = %d, want 410", dl2.StatusCode)
	}

	// View still allowed
	view := e.do(t, http.MethodGet, "/api/files/"+fileID, "", "", "")
	view.Body.Close()
	if view.StatusCode != http.StatusOK {
		t.Fatalf("view after cap = %d", view.StatusCode)
	}

	// View mode blocks download
	mode := e.doJSON(t, http.MethodPatch, "/api/files/"+fileID+"/share", cookie, map[string]any{
		"shareMode":    store.ShareModeView,
		"maxDownloads": 0, // clear cap so mode is the only block
	})
	mode.Body.Close()
	if mode.StatusCode != http.StatusOK {
		t.Fatalf("mode patch = %d", mode.StatusCode)
	}
	blocked := e.do(t, http.MethodGet, "/f/"+fileID+"?download=1", "", "", "")
	blocked.Body.Close()
	if blocked.StatusCode != http.StatusForbidden {
		t.Fatalf("view-mode download = %d, want 403", blocked.StatusCode)
	}
}

func TestShareRevokeAndExpiryCap(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "sharerev", "password1")

	up := e.do(t, http.MethodPost, "/api/upload?fileName=rev.txt", cookie, "revoke-me", "application/octet-stream")
	created := decodeJSON[map[string]any](t, up)
	fileID := created["fileId"].(string)

	far := e.now().Add(60 * 24 * time.Hour).Format(time.RFC3339)
	bad := e.doJSON(t, http.MethodPatch, "/api/files/"+fileID+"/share", cookie, map[string]any{
		"expiresAt": far,
	})
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("far expiry = %d, want 400", bad.StatusCode)
	}

	okExp := e.now().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	ok := e.doJSON(t, http.MethodPatch, "/api/files/"+fileID+"/share", cookie, map[string]any{
		"expiresAt": okExp,
		"password":  "temp-pass1",
	})
	ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("expiry patch = %d", ok.StatusCode)
	}

	rev := e.doJSON(t, http.MethodPost, "/api/files/"+fileID+"/revoke", cookie, map[string]any{})
	rev.Body.Close()
	if rev.StatusCode != http.StatusOK {
		t.Fatalf("revoke = %d", rev.StatusCode)
	}

	gone := e.do(t, http.MethodGet, "/api/files/"+fileID, "", "", "")
	gone.Body.Close()
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("after revoke = %d, want 404", gone.StatusCode)
	}
}

func TestShareUnlockCookieThenDownload(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "shareul", "password1")

	up := e.do(t, http.MethodPost, "/api/upload?fileName=ul.txt", cookie, "unlock-dl", "application/octet-stream")
	created := decodeJSON[map[string]any](t, up)
	fileID := created["fileId"].(string)

	patch := e.doJSON(t, http.MethodPatch, "/api/files/"+fileID+"/share", cookie, map[string]any{
		"password": "unlock-pass",
	})
	patch.Body.Close()

	unlock := e.doJSON(t, http.MethodPost, "/api/files/"+fileID+"/unlock", "", map[string]string{
		"password": "unlock-pass",
	})
	var unlockCookie string
	for _, c := range unlock.Cookies() {
		if strings.HasPrefix(c.Name, "discloud_unlock_") {
			unlockCookie = c.Name + "=" + c.Value
		}
	}
	unlock.Body.Close()
	if unlockCookie == "" {
		t.Fatal("missing unlock cookie")
	}

	req, _ := http.NewRequest(http.MethodGet, e.ts.URL+"/f/"+fileID+"?download=1", nil)
	req.Header.Set("Cookie", unlockCookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusOK {
		t.Fatalf("download after unlock = %d", resp.StatusCode)
	}
}
