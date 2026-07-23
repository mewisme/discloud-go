package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func createUploadSession(t *testing.T, baseURL, fileName string, fileSize int64) (uploadID, token string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"fileName": fileName,
		"fileSize": fileSize,
	})
	resp, err := http.Post(baseURL+"/api/uploads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create upload status = %d: %s", resp.StatusCode, b)
	}
	var out struct {
		UploadID    string `json:"uploadId"`
		ResumeToken string `json:"resumeToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.UploadID, out.ResumeToken
}

func putWithToken(t *testing.T, method, url, token string, body []byte, contentType string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set(uploadTokenHeader, token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestUploadSessionFlow(t *testing.T) {
	ts, _, _ := newTestServer(t)
	payload := []byte("hello-session-upload-world")
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])

	id, token := createUploadSession(t, ts.URL, "docs/Hello World.txt", int64(len(payload)))

	cr := postChunk(t, ts.URL, payload)
	if cr.Hash != hash {
		t.Fatalf("hash = %s, want %s", cr.Hash, hash)
	}

	regBody, _ := json.Marshal(map[string]string{"hash": hash})
	resp := putWithToken(t, http.MethodPut, ts.URL+"/api/uploads/"+id+"/parts/0", token, regBody, "application/json")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("register part = %d: %s", resp.StatusCode, b)
	}

	// Idempotent re-register.
	resp2 := putWithToken(t, http.MethodPut, ts.URL+"/api/uploads/"+id+"/parts/0", token, regBody, "application/json")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("re-register = %d", resp2.StatusCode)
	}

	comp := putWithToken(t, http.MethodPost, ts.URL+"/api/uploads/"+id+"/complete", token, nil, "")
	defer comp.Body.Close()
	if comp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(comp.Body)
		t.Fatalf("complete = %d: %s", comp.StatusCode, b)
	}
	var file map[string]any
	if err := json.NewDecoder(comp.Body).Decode(&file); err != nil {
		t.Fatal(err)
	}
	if file["fileName"] != "docs/hello-world.txt" {
		t.Fatalf("fileName = %v", file["fileName"])
	}
	fileID, _ := file["fileId"].(string)
	if fileID == "" {
		t.Fatal("missing fileId")
	}

	// Idempotent complete.
	comp2 := putWithToken(t, http.MethodPost, ts.URL+"/api/uploads/"+id+"/complete", token, nil, "")
	defer comp2.Body.Close()
	if comp2.StatusCode != http.StatusOK {
		t.Fatalf("complete again = %d", comp2.StatusCode)
	}
	var file2 map[string]any
	_ = json.NewDecoder(comp2.Body).Decode(&file2)
	if file2["fileId"] != fileID {
		t.Fatalf("idempotent fileId = %v, want %s", file2["fileId"], fileID)
	}
}

func TestUploadSessionCancel(t *testing.T) {
	ts, _, _ := newTestServer(t)
	id, token := createUploadSession(t, ts.URL, "a.bin", 100)
	resp := putWithToken(t, http.MethodDelete, ts.URL+"/api/uploads/"+id, token, nil, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("cancel = %d", resp.StatusCode)
	}
	get := putWithToken(t, http.MethodGet, ts.URL+"/api/uploads/"+id, token, nil, "")
	defer get.Body.Close()
	var prog map[string]any
	_ = json.NewDecoder(get.Body).Decode(&prog)
	if prog["status"] != "cancelled" {
		t.Fatalf("status = %v", prog["status"])
	}
	comp := putWithToken(t, http.MethodPost, ts.URL+"/api/uploads/"+id+"/complete", token, nil, "")
	defer comp.Body.Close()
	if comp.StatusCode != http.StatusGone {
		t.Fatalf("complete after cancel = %d, want 410", comp.StatusCode)
	}
}

func TestUploadSessionPartConflict(t *testing.T) {
	ts, _, _ := newTestServer(t)
	a := []byte("aaaaaaaa")
	b := []byte("bbbbbbbb")
	ha := sha256.Sum256(a)
	hb := sha256.Sum256(b)
	postChunk(t, ts.URL, a)
	postChunk(t, ts.URL, b)
	id, token := createUploadSession(t, ts.URL, "x.bin", 8)
	bodyA, _ := json.Marshal(map[string]string{"hash": hex.EncodeToString(ha[:])})
	resp := putWithToken(t, http.MethodPut, ts.URL+"/api/uploads/"+id+"/parts/0", token, bodyA, "application/json")
	resp.Body.Close()
	bodyB, _ := json.Marshal(map[string]string{"hash": hex.EncodeToString(hb[:])})
	resp2 := putWithToken(t, http.MethodPut, ts.URL+"/api/uploads/"+id+"/parts/0", token, bodyB, "application/json")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("conflict = %d: %s", resp2.StatusCode, b)
	}
}

func TestUploadSessionResumeMissing(t *testing.T) {
	ts, _, _ := newTestServer(t)
	payload := make([]byte, chunkSize+10)
	for i := range payload {
		payload[i] = byte(i)
	}
	c1, c2 := payload[:chunkSize], payload[chunkSize:]
	h1 := sha256.Sum256(c1)
	h2 := sha256.Sum256(c2)
	postChunk(t, ts.URL, c1)
	id, token := createUploadSession(t, ts.URL, "big.bin", int64(len(payload)))
	body, _ := json.Marshal(map[string]string{"hash": hex.EncodeToString(h1[:])})
	resp := putWithToken(t, http.MethodPut, ts.URL+"/api/uploads/"+id+"/parts/0", token, body, "application/json")
	resp.Body.Close()

	get := putWithToken(t, http.MethodGet, ts.URL+"/api/uploads/"+id, token, nil, "")
	defer get.Body.Close()
	var prog struct {
		Missing []int `json:"missingIndices"`
	}
	if err := json.NewDecoder(get.Body).Decode(&prog); err != nil {
		t.Fatal(err)
	}
	if len(prog.Missing) != 1 || prog.Missing[0] != 1 {
		t.Fatalf("missing = %v, want [1]", prog.Missing)
	}

	postChunk(t, ts.URL, c2)
	body2, _ := json.Marshal(map[string]any{
		"parts": []map[string]any{{"idx": 1, "hash": hex.EncodeToString(h2[:])}},
	})
	resp2 := putWithToken(t, http.MethodPost, ts.URL+"/api/uploads/"+id+"/parts", token, body2, "application/json")
	resp2.Body.Close()
	comp := putWithToken(t, http.MethodPost, ts.URL+"/api/uploads/"+id+"/complete", token, nil, "")
	defer comp.Body.Close()
	if comp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(comp.Body)
		t.Fatalf("complete = %d: %s", comp.StatusCode, b)
	}
}

func TestFormatFileNameRelative(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"folder/Sub Dir/File.TXT", "folder/sub-dir/file.txt"},
		{"../evil.txt", "evil.txt"},
		{"/abs/path.bin", "path.bin"},
		{"a/b/c/d.txt", "a/b/c/d.txt"},
	}
	for _, tc := range cases {
		got := formatFileName(tc.in)
		if got != tc.want {
			t.Errorf("formatFileName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestUploadSessionRequiresToken(t *testing.T) {
	ts, _, _ := newTestServer(t)
	id, _ := createUploadSession(t, ts.URL, "x.bin", 10)
	resp, err := http.Get(ts.URL + "/api/uploads/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	_ = fmt.Sprintf("ok")
}

func TestUploadSessionExpire(t *testing.T) {
	e := newAuthEnv(t)
	id, token := createUploadSession(t, e.ts.URL, "expire.bin", 100)
	e.setNow(e.now().Add(uploadSessionAnonTTL + time.Hour))
	n, err := e.st.ExpireUploadSessions(context.Background(), e.now(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expired count = %d, want 1", n)
	}
	comp := putWithToken(t, http.MethodPost, e.ts.URL+"/api/uploads/"+id+"/complete", token, nil, "")
	defer comp.Body.Close()
	if comp.StatusCode != http.StatusGone {
		b, _ := io.ReadAll(comp.Body)
		t.Fatalf("complete after expire = %d: %s, want 410", comp.StatusCode, b)
	}
}
