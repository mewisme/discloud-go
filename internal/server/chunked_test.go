package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"testing"
)

type chunkResp struct {
	Hash    string `json:"hash"`
	Existed bool   `json:"existed"`
}

func postChunk(t *testing.T, baseURL string, data []byte) chunkResp {
	t.Helper()
	resp, err := http.Post(baseURL+"/api/chunks", "application/octet-stream", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("chunk upload status = %d: %s", resp.StatusCode, body)
	}
	var cr chunkResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		t.Fatal(err)
	}
	return cr
}

func completeUpload(t *testing.T, baseURL, fileName string, hashes []string) (*http.Response, []byte) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"fileName": fileName, "chunkHashes": hashes})
	resp, err := http.Post(baseURL+"/api/upload/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp, respBody
}

func TestChunkedUploadFlow(t *testing.T) {
	ts, fake, _ := newTestServer(t)

	// One full 8 MB chunk plus a short tail.
	payload := make([]byte, chunkSize+4096)
	rand.New(rand.NewSource(7)).Read(payload)
	chunk1, chunk2 := payload[:chunkSize], payload[chunkSize:]

	sum1 := sha256.Sum256(chunk1)
	wantHash1 := hex.EncodeToString(sum1[:])

	// Unknown chunk reports 404 before upload.
	resp, err := http.Get(ts.URL + "/api/chunks/" + wantHash1)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("pre-upload check status = %d, want 404", resp.StatusCode)
	}

	// Upload both chunks; server must compute the same hash the client did.
	cr1 := postChunk(t, ts.URL, chunk1)
	if cr1.Hash != wantHash1 || cr1.Existed {
		t.Fatalf("chunk1 = %+v, want hash %s existed=false", cr1, wantHash1)
	}
	cr2 := postChunk(t, ts.URL, chunk2)

	// Now the check endpoint finds it.
	resp, err = http.Get(ts.URL + "/api/chunks/" + cr1.Hash)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post-upload check status = %d, want 200", resp.StatusCode)
	}

	// Re-uploading the same chunk (a retried upload) skips Discord entirely.
	uploadsBefore := fake.uploads
	if cr := postChunk(t, ts.URL, chunk1); !cr.Existed {
		t.Error("re-upload of identical chunk should report existed=true")
	}
	if fake.uploads != uploadsBefore {
		t.Errorf("discord uploads grew from %d to %d on duplicate chunk", uploadsBefore, fake.uploads)
	}

	// Assemble the file and verify the download byte-for-byte.
	resp2, body := completeUpload(t, ts.URL, "Chunked Upload.bin", []string{cr1.Hash, cr2.Hash})
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("complete status = %d: %s", resp2.StatusCode, body)
	}
	var up struct {
		FileID   string `json:"fileId"`
		FileName string `json:"fileName"`
		FileSize int64  `json:"fileSize"`
	}
	if err := json.Unmarshal(body, &up); err != nil {
		t.Fatal(err)
	}
	if up.FileSize != int64(len(payload)) {
		t.Errorf("fileSize = %d, want %d", up.FileSize, len(payload))
	}
	if up.FileName != "chunked-upload.bin" {
		t.Errorf("fileName = %q, want %q", up.FileName, "chunked-upload.bin")
	}

	dl, err := http.Get(ts.URL + "/f/" + up.FileID)
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	got, err := io.ReadAll(dl.Body)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(got) != sha256.Sum256(payload) {
		t.Fatalf("downloaded bytes differ from source (%d vs %d bytes)", len(got), len(payload))
	}
}

func TestChunkedUploadValidation(t *testing.T) {
	ts, _, _ := newTestServer(t)

	small := []byte("tiny chunk")
	cr := postChunk(t, ts.URL, small)

	// A short chunk anywhere but last is rejected.
	full := make([]byte, chunkSize)
	crFull := postChunk(t, ts.URL, full)
	resp, body := completeUpload(t, ts.URL, "bad.bin", []string{cr.Hash, crFull.Hash})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("short middle chunk: status = %d, want 400 (%s)", resp.StatusCode, body)
	}

	// Unknown hash is rejected.
	unknown := strings.Repeat("ab", 32)
	resp, _ = completeUpload(t, ts.URL, "bad.bin", []string{unknown})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown hash: status = %d, want 400", resp.StatusCode)
	}

	// Malformed hash is rejected.
	resp, _ = completeUpload(t, ts.URL, "bad.bin", []string{"not-a-hash"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed hash: status = %d, want 400", resp.StatusCode)
	}

	// Empty chunk list and missing name are rejected.
	resp, _ = completeUpload(t, ts.URL, "bad.bin", []string{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty hashes: status = %d, want 400", resp.StatusCode)
	}
	respRaw, err := http.Post(ts.URL+"/api/upload/complete", "application/json",
		strings.NewReader(fmt.Sprintf(`{"chunkHashes":[%q]}`, cr.Hash)))
	if err != nil {
		t.Fatal(err)
	}
	respRaw.Body.Close()
	if respRaw.StatusCode != http.StatusBadRequest {
		t.Errorf("missing fileName: status = %d, want 400", respRaw.StatusCode)
	}

	// Empty chunk body is rejected.
	respRaw, err = http.Post(ts.URL+"/api/chunks", "application/octet-stream", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	respRaw.Body.Close()
	if respRaw.StatusCode != http.StatusBadRequest {
		t.Errorf("empty chunk: status = %d, want 400", respRaw.StatusCode)
	}

	// Oversized chunk is rejected.
	big := make([]byte, chunkSize+1)
	respRaw, err = http.Post(ts.URL+"/api/chunks", "application/octet-stream", bytes.NewReader(big))
	if err != nil {
		t.Fatal(err)
	}
	respRaw.Body.Close()
	if respRaw.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized chunk: status = %d, want 413", respRaw.StatusCode)
	}
}
