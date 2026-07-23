package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestUploadChunkedSkipsExisting(t *testing.T) {
	var mu sync.Mutex
	stored := map[string][]byte{}
	var completeHashes []string
	var posts int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chunkSize": 4,
			})
		case r.Method == http.MethodHead && len(r.URL.Path) > len("/api/chunks/"):
			hash := r.URL.Path[len("/api/chunks/"):]
			mu.Lock()
			_, ok := stored[hash]
			mu.Unlock()
			if ok {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/chunks":
			data, _ := io.ReadAll(r.Body)
			sum := sha256.Sum256(data)
			hash := hex.EncodeToString(sum[:])
			mu.Lock()
			posts++
			stored[hash] = append([]byte(nil), data...)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"hash": hash, "existed": false})
		case r.Method == http.MethodPost && r.URL.Path == "/api/upload/complete":
			var body struct {
				FileName    string   `json:"fileName"`
				ChunkHashes []string `json:"chunkHashes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			completeHashes = append([]string(nil), body.ChunkHashes...)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"fileId": "abc", "fileName": body.FileName,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "blob.bin")
	// 10 bytes → 3 chunks of size 4 (4+4+2)
	content := []byte("0123456789")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := New(Config{
		BaseURL:    srv.URL,
		Origin:     "http://localhost:3000",
		CookiePath: filepath.Join(dir, "cookies"),
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := c.UploadChunked(path, UploadChunkedOptions{FileName: "blob.bin", Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if out["fileId"] != "abc" {
		t.Fatalf("unexpected response: %v", out)
	}
	if len(completeHashes) != 3 {
		t.Fatalf("want 3 hashes, got %d (%v)", len(completeHashes), completeHashes)
	}
	if posts != 3 {
		t.Fatalf("first upload posts=%d want 3", posts)
	}

	// Second upload: all chunks exist → zero POSTs.
	mu.Lock()
	posts = 0
	mu.Unlock()
	_, err = c.UploadChunked(path, UploadChunkedOptions{FileName: "blob.bin", Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	n := posts
	mu.Unlock()
	if n != 0 {
		t.Fatalf("resume upload posts=%d want 0", n)
	}
}

func TestUploadChunkedSession(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("APPDATA", cfgDir)
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	var mu sync.Mutex
	stored := map[string][]byte{}
	parts := map[int]string{}
	var partPuts int
	var creates int
	uploadID := "11111111-1111-1111-1111-111111111111"
	resumeToken := "resume-tok"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/info":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chunkSize": 4,
				"uploads":   map[string]any{"sessions": true},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/uploads":
			mu.Lock()
			creates++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uploadId": uploadID, "resumeToken": resumeToken,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/uploads/"+uploadID:
			mu.Lock()
			missing := []int{}
			for i := 0; i < 3; i++ {
				if parts[i] == "" {
					missing = append(missing, i)
				}
			}
			status := "pending"
			if len(parts) > 0 {
				status = "uploading"
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": status, "missingIndices": missing, "parts": parts,
			})
		case r.Method == http.MethodPut && len(r.URL.Path) > len("/api/uploads/"+uploadID+"/parts/"):
			idxStr := r.URL.Path[len("/api/uploads/"+uploadID+"/parts/"):]
			idx, _ := strconv.Atoi(idxStr)
			var body struct {
				Hash string `json:"hash"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			partPuts++
			parts[idx] = body.Hash
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/uploads/"+uploadID+"/complete":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"fileId": "sess-file", "fileName": "blob.bin",
			})
		case r.Method == http.MethodHead && len(r.URL.Path) > len("/api/chunks/"):
			hash := r.URL.Path[len("/api/chunks/"):]
			mu.Lock()
			_, ok := stored[hash]
			mu.Unlock()
			if ok {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/chunks":
			data, _ := io.ReadAll(r.Body)
			sum := sha256.Sum256(data)
			hash := hex.EncodeToString(sum[:])
			mu.Lock()
			stored[hash] = append([]byte(nil), data...)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"hash": hash, "existed": false})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "blob.bin")
	content := []byte("0123456789") // 3 chunks of 4
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{
		BaseURL:    srv.URL,
		Origin:     "http://localhost:3000",
		CookiePath: filepath.Join(dir, "cookies"),
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := c.UploadChunked(path, UploadChunkedOptions{FileName: "blob.bin", Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if out["fileId"] != "sess-file" {
		t.Fatalf("unexpected: %v", out)
	}
	mu.Lock()
	if creates != 1 || partPuts != 3 {
		t.Fatalf("creates=%d partPuts=%d", creates, partPuts)
	}
	mu.Unlock()

	// Preload checkpoint as if interrupted after part 0.
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	size, mtime := fileFingerprint(st)
	abs, _ := filepath.Abs(path)
	h0 := sha256.Sum256(content[:4])
	mu.Lock()
	parts = map[int]string{0: hex.EncodeToString(h0[:])}
	partPuts = 0
	creates = 0
	mu.Unlock()
	if err := saveCheckpoint(&uploadCheckpoint{
		UploadID: uploadID, ResumeToken: resumeToken, Path: abs,
		Size: size, ModTimeUnix: mtime, ChunkSize: 4, FileName: "blob.bin",
		Hashes: map[int]string{0: hex.EncodeToString(h0[:])},
	}); err != nil {
		t.Fatal(err)
	}

	_, err = c.UploadChunked(path, UploadChunkedOptions{FileName: "blob.bin", Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if creates != 0 {
		t.Fatalf("resume should reuse session, creates=%d", creates)
	}
	if partPuts != 2 {
		t.Fatalf("resume partPuts=%d want 2 (skip idx 0)", partPuts)
	}
}

func TestAbortUpload(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("APPDATA", cfgDir)
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	var deleted bool
	uploadID := "22222222-2222-2222-2222-222222222222"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/uploads/"+uploadID {
			if r.Header.Get("X-Upload-Token") != "tok" {
				http.Error(w, "missing token", 404)
				return
			}
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	size, mtime := fileFingerprint(st)
	abs, _ := filepath.Abs(path)
	if err := saveCheckpoint(&uploadCheckpoint{
		UploadID: uploadID, ResumeToken: "tok", Path: abs,
		Size: size, ModTimeUnix: mtime, ChunkSize: 4, FileName: "f.bin",
		Hashes: map[int]string{},
	}); err != nil {
		t.Fatal(err)
	}

	c, err := New(Config{
		BaseURL:    srv.URL,
		Origin:     "http://localhost:3000",
		CookiePath: filepath.Join(dir, "cookies"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AbortUpload(path); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected DELETE")
	}
	if _, err := loadCheckpoint(path, size, mtime); err == nil {
		t.Fatal("checkpoint should be cleared")
	}
}
