package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mewisme/discloud-go/internal/store"
)

func TestAdminOverviewRequiresAdmin(t *testing.T) {
	e := newAuthEnv(t)
	cAdmin, _ := e.signup(t, "admin1", "password1")
	cUser, _ := e.signup(t, "user2", "password2")

	anon := e.do(t, http.MethodGet, "/api/admin/overview", "", "", "")
	if anon.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(anon.Body)
		anon.Body.Close()
		t.Fatalf("anon status = %d: %s", anon.StatusCode, body)
	}
	anon.Body.Close()

	denied := e.do(t, http.MethodGet, "/api/admin/overview", cUser, "", "")
	if denied.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(denied.Body)
		denied.Body.Close()
		t.Fatalf("user status = %d: %s", denied.StatusCode, body)
	}
	denied.Body.Close()

	ok := e.do(t, http.MethodGet, "/api/admin/overview", cAdmin, "", "")
	if ok.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(ok.Body)
		ok.Body.Close()
		t.Fatalf("admin status = %d: %s", ok.StatusCode, body)
	}
	var body map[string]any
	if err := json.NewDecoder(ok.Body).Decode(&body); err != nil {
		ok.Body.Close()
		t.Fatal(err)
	}
	ok.Body.Close()

	for _, key := range []string{"storage", "users", "uploads", "traffic", "bots", "deps"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing key %q in %+v", key, body)
		}
	}
	users, _ := body["users"].(map[string]any)
	if users["count"] != float64(2) || users["admins"] != float64(1) {
		t.Fatalf("users = %+v", users)
	}
	bots, _ := body["bots"].(map[string]any)
	if bots["configured"] != float64(1) {
		t.Fatalf("bots = %+v, want configured=1", bots)
	}
	deps, _ := body["deps"].(map[string]any)
	if deps["postgres"] != true || deps["valkey"] != true {
		t.Fatalf("deps = %+v", deps)
	}
}

// Admin access matrix:
//  1. admin cookie → OK
//  2. admin cookie + Bearer PAT without admin scope → OK (cookie wins)
//  3. no cookie + Bearer without admin scope → 403
//  4. no cookie + Bearer with admin scope (admin role) → OK
func TestAdminOverviewAuthMatrix(t *testing.T) {
	e := newAuthEnv(t)
	cookie, _ := e.signup(t, "admin1", "password1")

	patNoAdmin := e.createPAT(t, cookie, "noadmin", []string{
		store.ScopeUpload, store.ScopeRead, store.ScopeManage,
	}, nil)
	rawNo, _ := patNoAdmin["token"].(string)
	if !strings.HasPrefix(rawNo, "dc_") {
		t.Fatalf("token = %q", rawNo)
	}

	patAdmin := e.createPAT(t, cookie, "withadmin", []string{
		store.ScopeAdmin, store.ScopeManage, store.ScopeRead, store.ScopeUpload,
	}, nil)
	rawYes, _ := patAdmin["token"].(string)

	// 1. admin session cookie
	r1 := e.do(t, http.MethodGet, "/api/admin/overview", cookie, "", "")
	if r1.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r1.Body)
		r1.Body.Close()
		t.Fatalf("1 cookie admin = %d: %s", r1.StatusCode, b)
	}
	r1.Body.Close()

	// 2. cookie + PAT without admin → cookie wins
	req, err := http.NewRequest(http.MethodGet, e.ts.URL+"/api/admin/overview", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", sessionCookieName+"="+cookie)
	req.Header.Set("Authorization", "Bearer "+rawNo)
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if r2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r2.Body)
		r2.Body.Close()
		t.Fatalf("2 cookie+PAT(no admin) = %d: %s", r2.StatusCode, b)
	}
	r2.Body.Close()

	// 3. logout equivalent: Bearer only, no admin scope
	r3 := e.doBearer(t, http.MethodGet, "/api/admin/overview", rawNo, "", "")
	if r3.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(r3.Body)
		r3.Body.Close()
		t.Fatalf("3 PAT without admin = %d: %s", r3.StatusCode, b)
	}
	r3.Body.Close()

	// 4. Bearer with admin scope
	r4 := e.doBearer(t, http.MethodGet, "/api/admin/overview", rawYes, "", "")
	if r4.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r4.Body)
		r4.Body.Close()
		t.Fatalf("4 PAT with admin = %d: %s", r4.StatusCode, b)
	}
	r4.Body.Close()
}
