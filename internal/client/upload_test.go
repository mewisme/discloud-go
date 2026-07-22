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
