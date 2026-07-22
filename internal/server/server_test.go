package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/store"
)

// fakeDiscord emulates the two Discord API endpoints the app uses plus the
// CDN, all in one httptest server.
type fakeDiscord struct {
	mu       sync.Mutex
	messages map[string][]byte // message id -> chunk bytes
	nextID   int
	baseURL  string
	uploads  int
}

func newFakeDiscord(t *testing.T) (*fakeDiscord, *httptest.Server) {
	f := &fakeDiscord{messages: map[string][]byte{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /channels/{ch}/messages", func(w http.ResponseWriter, r *http.Request) {
		file, _, err := r.FormFile("files[0]")
		if err != nil {
			t.Errorf("fake discord: bad multipart: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		data, _ := io.ReadAll(file)
		f.mu.Lock()
		f.nextID++
		id := strconv.Itoa(f.nextID)
		f.messages[id] = data
		f.uploads++
		f.mu.Unlock()
		json.NewEncoder(w).Encode(f.messageJSON(id))
	})
	mux.HandleFunc("GET /channels/{ch}/messages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		f.mu.Lock()
		_, ok := f.messages[id]
		f.mu.Unlock()
		if !ok {
			http.Error(w, "unknown message", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(f.messageJSON(id))
	})
	mux.HandleFunc("GET /attachments/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		data, ok := f.messages[r.PathValue("id")]
		f.mu.Unlock()
		if !ok {
			http.Error(w, "gone", http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, "chunk", time.Time{}, bytes.NewReader(data))
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	f.baseURL = ts.URL
	return f, ts
}

func (f *fakeDiscord) messageJSON(id string) map[string]any {
	ex := strconv.FormatInt(time.Now().Add(24*time.Hour).Unix(), 16)
	return map[string]any{
		"id": id,
		"attachments": []map[string]string{
			{"url": fmt.Sprintf("%s/attachments/%s?ex=%s", f.baseURL, id, ex)},
		},
	}
}

type memStore struct {
	mu       sync.Mutex
	files    map[string]store.File
	chunks   map[string]store.Chunk
	stats    map[string]*fileStats
	visitors map[string]map[string]struct{}
}

type fileStats struct {
	views, downloads, ranges, bytesServed, uniqueVisitors int64
	lastAccess                                            time.Time
}

func (m *memStore) ensureStats(id string) *fileStats {
	if m.stats == nil {
		m.stats = map[string]*fileStats{}
	}
	if m.stats[id] == nil {
		m.stats[id] = &fileStats{}
	}
	return m.stats[id]
}

func (m *memStore) CreateFile(_ context.Context, f store.File) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[f.ID] = f
	m.ensureStats(f.ID)
	return nil
}

func (m *memStore) GetFile(_ context.Context, id string) (store.File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[id]
	if !ok {
		return store.File{}, store.ErrNotFound
	}
	return f, nil
}

func (m *memStore) ListFiles(_ context.Context, limit int) ([]store.File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	files := make([]store.File, 0, len(m.files))
	for _, f := range m.files {
		files = append(files, f)
	}
	if len(files) > limit {
		files = files[:limit]
	}
	return files, nil
}

func (m *memStore) HasChunk(_ context.Context, hash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.chunks[hash]
	return ok, nil
}

func (m *memStore) PutChunk(_ context.Context, c store.Chunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.chunks[c.Hash]; !ok {
		m.chunks[c.Hash] = c
	}
	return nil
}

func (m *memStore) GetChunks(_ context.Context, hashes []string) (map[string]store.Chunk, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]store.Chunk{}
	for _, h := range hashes {
		if c, ok := m.chunks[h]; ok {
			out[h] = c
		}
	}
	return out, nil
}

func (m *memStore) DeleteChunksByMessageID(_ context.Context, messageID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for h, c := range m.chunks {
		if c.MessageID == messageID {
			delete(m.chunks, h)
		}
	}
	return nil
}

func (m *memStore) EnsureBots(context.Context, int) error { return nil }

func (m *memStore) RecordEvent(_ context.Context, e store.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[e.FileID]; !ok {
		return store.ErrNotFound
	}
	st := m.ensureStats(e.FileID)
	switch e.Kind {
	case store.EventView:
		st.views++
	case store.EventDownload:
		st.downloads++
	case store.EventRange:
		st.ranges++
	}
	st.bytesServed += e.Bytes
	st.lastAccess = time.Now().UTC()
	if e.VisitorHash != "" {
		if m.visitors == nil {
			m.visitors = map[string]map[string]struct{}{}
		}
		if m.visitors[e.FileID] == nil {
			m.visitors[e.FileID] = map[string]struct{}{}
		}
		if _, ok := m.visitors[e.FileID][e.VisitorHash]; !ok {
			m.visitors[e.FileID][e.VisitorHash] = struct{}{}
			st.uniqueVisitors++
		}
	}
	return nil
}

func (m *memStore) GetFileInspect(_ context.Context, id string) (store.FileInspect, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[id]
	if !ok {
		return store.FileInspect{}, store.ErrNotFound
	}
	st := m.ensureStats(id)
	var last *time.Time
	if !st.lastAccess.IsZero() {
		t := st.lastAccess
		last = &t
	}
	return store.FileInspect{
		File: f, Views: st.views, Downloads: st.downloads,
		Ranges: st.ranges, BytesServed: st.bytesServed, UniqueVisitors: st.uniqueVisitors,
		LastAccessAt: last, ChunkCount: len(f.Parts),
	}, nil
}

func (m *memStore) Ping(context.Context) error { return nil }

type memCache struct {
	mu   sync.Mutex
	urls map[string]string
	hits int
}

func (c *memCache) GetURL(_ context.Context, id string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	u, ok := c.urls[id]
	if ok {
		c.hits++
	}
	return u, ok
}

func (c *memCache) SetURL(_ context.Context, id, u string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.urls[id] = u
}

func (c *memCache) Ping(context.Context) error { return nil }

func newTestServer(t *testing.T) (*httptest.Server, *fakeDiscord, *memCache) {
	return newTestServerWithTokens(t, "test-token")
}

func newTestServerWithTokens(t *testing.T, tokens string) (*httptest.Server, *fakeDiscord, *memCache) {
	t.Helper()
	fake, discordTS := newFakeDiscord(t)
	dc := discord.New(tokens, "test-channel")
	dc.BaseURL = discordTS.URL
	ca := &memCache{urls: map[string]string{}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(log, &memStore{files: map[string]store.File{}, chunks: map[string]store.Chunk{}}, ca, dc, "", "test-salt")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, fake, ca
}

func TestInfoWorkersScaleWithBots(t *testing.T) {
	ts1, _, _ := newTestServerWithTokens(t, "tok-a")
	resp, err := http.Get(ts1.URL + "/api/info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var one struct {
		Bots    int `json:"bots"`
		Workers int `json:"workers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&one); err != nil {
		t.Fatal(err)
	}
	if one.Bots != 1 || one.Workers != singleBotUploadConcurrency {
		t.Fatalf("single bot info = %+v, want bots=1 workers=%d", one, singleBotUploadConcurrency)
	}

	ts3, _, _ := newTestServerWithTokens(t, "tok-a,tok-b,tok-c")
	resp3, err := http.Get(ts3.URL + "/api/info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	var three struct {
		Bots    int `json:"bots"`
		Workers int `json:"workers"`
	}
	if err := json.NewDecoder(resp3.Body).Decode(&three); err != nil {
		t.Fatal(err)
	}
	if three.Bots != 3 || three.Workers != 3 {
		t.Fatalf("multi bot info = %+v, want bots=3 workers=3", three)
	}
}

func TestUploadLinksUseAPIURL(t *testing.T) {
	fake, discordTS := newFakeDiscord(t)
	dc := discord.New("test-token", "test-channel")
	dc.BaseURL = discordTS.URL
	ca := &memCache{urls: map[string]string{}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	const public = "https://files.example.com"
	srv := New(log, &memStore{files: map[string]store.File{}, chunks: map[string]store.Chunk{}}, ca, dc, public, "test-salt")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Post(ts.URL+"/api/upload?fileName=a.txt", "application/octet-stream", strings.NewReader("hi"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var up struct {
		URL             string `json:"url"`
		LongURL         string `json:"longURL"`
		DownloadURL     string `json:"downloadURL"`
		LongDownloadURL string `json:"longDownloadURL"`
		FileID          string `json:"fileId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&up); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(up.URL, public+"/") {
		t.Fatalf("url = %q, want prefix %q (not request host %q)", up.URL, public, ts.URL)
	}
	wantLong := public + "/f/" + up.FileID + "/a.txt"
	if up.LongURL != wantLong {
		t.Fatalf("longURL = %q, want %q", up.LongURL, wantLong)
	}
	if !strings.HasPrefix(up.DownloadURL, public+"/") || !strings.HasPrefix(up.LongDownloadURL, public+"/") {
		t.Fatalf("download URLs missing public base: %+v", up)
	}
	_ = fake
}

func TestUploadDownloadRoundTrip(t *testing.T) {
	ts, fake, ca := newTestServer(t)

	// 20 MB + change: exercises full chunks plus a short tail chunk.
	payload := make([]byte, 20<<20+12345)
	rand.New(rand.NewSource(42)).Read(payload)

	resp, err := http.Post(ts.URL+"/api/upload?fileName="+url.QueryEscape("Round Trip!.bin"), "application/octet-stream", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d: %s", resp.StatusCode, body)
	}
	var up struct {
		FileID   string `json:"fileId"`
		FileName string `json:"fileName"`
		FileSize int64  `json:"fileSize"`
		URL      string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&up); err != nil {
		t.Fatal(err)
	}
	if up.FileSize != int64(len(payload)) {
		t.Errorf("fileSize = %d, want %d", up.FileSize, len(payload))
	}
	if up.FileName != "round-trip.bin" {
		t.Errorf("fileName = %q, want %q", up.FileName, "round-trip.bin")
	}
	if fake.uploads != 3 {
		t.Errorf("discord uploads = %d, want 3", fake.uploads)
	}

	// Full download must be byte-identical.
	dl, err := http.Get(ts.URL + "/f/" + up.FileID)
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	got, err := io.ReadAll(dl.Body)
	if err != nil {
		t.Fatal(err)
	}
	if dl.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d", dl.StatusCode)
	}
	if sha256.Sum256(got) != sha256.Sum256(payload) {
		t.Fatalf("downloaded bytes differ from upload (%d vs %d bytes)", len(got), len(payload))
	}

	// Range download crossing the 8 MB chunk boundary.
	start, end := int64(8<<20-100), int64(8<<20+100)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/f/"+up.FileID, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	pr, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Body.Close()
	if pr.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", pr.StatusCode)
	}
	wantCR := fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))
	if cr := pr.Header.Get("Content-Range"); cr != wantCR {
		t.Errorf("Content-Range = %q, want %q", cr, wantCR)
	}
	part, _ := io.ReadAll(pr.Body)
	if !bytes.Equal(part, payload[start:end+1]) {
		t.Fatal("range bytes differ from source slice")
	}

	// Second download should hit the URL cache instead of the Discord API.
	if ca.hits == 0 {
		t.Error("expected cache hits on second download, got none")
	}

	// File list includes the upload.
	lr, err := http.Get(ts.URL + "/api/files")
	if err != nil {
		t.Fatal(err)
	}
	defer lr.Body.Close()
	var list struct {
		Files []store.File `json:"files"`
	}
	if err := json.NewDecoder(lr.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Files) != 1 || list.Files[0].ID != up.FileID {
		t.Errorf("list = %+v, want the uploaded file", list.Files)
	}
}

func TestUploadValidation(t *testing.T) {
	ts, _, _ := newTestServer(t)

	resp, err := http.Post(ts.URL+"/api/upload", "application/octet-stream", strings.NewReader("data"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing fileName: status = %d, want 400", resp.StatusCode)
	}

	resp, err = http.Post(ts.URL+"/api/upload?fileName=x.txt", "application/octet-stream", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty body: status = %d, want 400", resp.StatusCode)
	}
}

func TestDownloadNotFound(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/f/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSingleChunkRedirectsToCDN(t *testing.T) {
	ts, _, _ := newTestServer(t)
	payload := []byte("tiny file for redirect")
	resp, err := http.Post(ts.URL+"/api/upload?fileName=tiny.txt",
		"application/octet-stream", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var up struct {
		FileID string `json:"fileId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&up); err != nil {
		t.Fatal(err)
	}

	noFollow := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	redir, err := noFollow.Get(ts.URL + "/f/" + up.FileID)
	if err != nil {
		t.Fatal(err)
	}
	defer redir.Body.Close()
	if redir.StatusCode != http.StatusFound {
		t.Fatalf("inline status = %d, want 302", redir.StatusCode)
	}
	loc := redir.Header.Get("Location")
	if !strings.Contains(loc, "/attachments/") {
		t.Fatalf("Location = %q, want Discord CDN attachment URL", loc)
	}

	// ?download=1 keeps proxy so Content-Disposition filename works.
	dl, err := noFollow.Get(ts.URL + "/f/" + up.FileID + "?download=1")
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	if dl.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d, want 200 (proxied)", dl.StatusCode)
	}
	got, _ := io.ReadAll(dl.Body)
	if !bytes.Equal(got, payload) {
		t.Fatalf("download body = %q, want %q", got, payload)
	}
}

func TestDownloadJSON(t *testing.T) {
	ts, _, _ := newTestServer(t)
	payload := []byte("hello json meta")
	resp, err := http.Post(ts.URL+"/api/upload?fileName=meta.txt",
		"application/octet-stream", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var up struct {
		FileID string `json:"fileId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&up); err != nil {
		t.Fatal(err)
	}

	meta, err := http.Get(ts.URL + "/f/" + up.FileID + "?json=1")
	if err != nil {
		t.Fatal(err)
	}
	defer meta.Body.Close()
	if meta.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", meta.StatusCode)
	}
	if ct := meta.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want json", ct)
	}
	var body struct {
		FileID   string `json:"fileId"`
		FileName string `json:"fileName"`
		FileSize int64  `json:"fileSize"`
	}
	if err := json.NewDecoder(meta.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.FileID != up.FileID || body.FileName != "meta.txt" || body.FileSize != int64(len(payload)) {
		t.Fatalf("meta = %+v", body)
	}

	missing, err := http.Get(ts.URL + "/f/nope?json=1")
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", missing.StatusCode)
	}
}

func TestInspectTracking(t *testing.T) {
	ts, _, _ := newTestServer(t)
	payload := []byte("track me please!!")
	resp, err := http.Post(ts.URL+"/api/upload?fileName=track.bin",
		"application/octet-stream", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	var up struct {
		FileID string `json:"fileId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&up); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	view, err := http.Get(ts.URL + "/f/" + up.FileID)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, view.Body)
	view.Body.Close()

	dl, err := http.Get(ts.URL + "/f/" + up.FileID + "?download=1")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, dl.Body)
	dl.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/f/"+up.FileID, nil)
	req.Header.Set("Range", "bytes=0-3")
	rr, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, rr.Body)
	rr.Body.Close()

	// Async track — wait briefly for goroutines.
	deadline := time.Now().Add(2 * time.Second)
	var insp struct {
		Views      int64 `json:"views"`
		Downloads  int64 `json:"downloads"`
		Ranges     int64 `json:"ranges"`
		ChunkCount int   `json:"chunkCount"`
	}
	for {
		ir, err := http.Get(ts.URL + "/api/files/" + up.FileID + "/inspect")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewDecoder(ir.Body).Decode(&insp); err != nil {
			ir.Body.Close()
			t.Fatal(err)
		}
		ir.Body.Close()
		if insp.Views >= 1 && insp.Downloads >= 1 && insp.Ranges >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("counters not updated: %+v", insp)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if insp.ChunkCount < 1 {
		t.Fatalf("chunkCount = %d", insp.ChunkCount)
	}
}
