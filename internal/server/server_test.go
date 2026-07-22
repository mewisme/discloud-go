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
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mewisme/discloud-go/internal/auth"
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
	// Deletion/expiration is Postgres-only — Discord must never be asked to delete.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Errorf("fake discord: unexpected DELETE %s (must not delete Discord messages)", r.URL.Path)
			http.Error(w, "delete not allowed", http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
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
	users    map[string]store.User
	byUser   map[string]string        // username -> user id
	sessions map[string]store.Session // token hash -> session
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
	if f.Visibility == "" {
		f.Visibility = store.VisibilityPublic
	}
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

func (m *memStore) ListFilesByOwner(_ context.Context, ownerID string, limit, offset int) ([]store.File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	files := make([]store.File, 0)
	for _, f := range m.files {
		if f.OwnerUserID != nil && *f.OwnerUserID == ownerID {
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].CreatedAt.Equal(files[j].CreatedAt) {
			return files[i].ID > files[j].ID
		}
		return files[i].CreatedAt.After(files[j].CreatedAt)
	})
	if offset > len(files) {
		return []store.File{}, nil
	}
	files = files[offset:]
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

func (m *memStore) CreateUser(_ context.Context, id, username, passwordHash string) (store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.users == nil {
		m.users = map[string]store.User{}
		m.byUser = map[string]string{}
	}
	if _, ok := m.byUser[username]; ok {
		return store.User{}, store.ErrConflict
	}
	role := store.RoleUser
	if len(m.users) == 0 {
		role = store.RoleAdmin
	}
	now := time.Now().UTC()
	u := store.User{
		ID: id, Username: username, PasswordHash: passwordHash, Role: role,
		DefaultVisibility: store.VisibilityPublic,
		CreatedAt:         now, UpdatedAt: now, PasswordChangedAt: now,
	}
	m.users[id] = u
	m.byUser[username] = id
	return u, nil
}

func (m *memStore) GetUserByUsername(_ context.Context, username string) (store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byUser[username]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return m.users[id], nil
}

func (m *memStore) GetUserByID(_ context.Context, id string) (store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}

func (m *memStore) CreateSession(_ context.Context, sess store.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions == nil {
		m.sessions = map[string]store.Session{}
	}
	if sess.LastSeenAt.IsZero() {
		sess.LastSeenAt = sess.CreatedAt
	}
	m.sessions[sess.TokenHash] = sess
	return nil
}

func (m *memStore) GetUserBySessionHash(_ context.Context, tokenHash string, now time.Time) (store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[tokenHash]
	if !ok || !sess.ExpiresAt.After(now) {
		return store.User{}, store.ErrNotFound
	}
	u, ok := m.users[sess.UserID]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}

func (m *memStore) GetSessionByTokenHash(_ context.Context, tokenHash string, now time.Time) (store.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[tokenHash]
	if !ok || !sess.ExpiresAt.After(now) {
		return store.Session{}, store.ErrNotFound
	}
	return sess, nil
}

func (m *memStore) TouchSession(_ context.Context, tokenHash, ip, userAgent string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[tokenHash]
	if !ok || !sess.ExpiresAt.After(now) {
		return store.ErrNotFound
	}
	sess.LastSeenAt = now
	sess.IP = ip
	sess.UserAgent = userAgent
	m.sessions[tokenHash] = sess
	return nil
}

func (m *memStore) DeleteSession(_ context.Context, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, tokenHash)
	return nil
}

func (m *memStore) DeleteSessionsByUserID(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for hash, sess := range m.sessions {
		if sess.UserID == userID {
			delete(m.sessions, hash)
		}
	}
	return nil
}

func (m *memStore) UpdatePasswordHash(_ context.Context, userID, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[userID]
	if !ok {
		return store.ErrNotFound
	}
	now := time.Now().UTC()
	u.PasswordHash = passwordHash
	u.UpdatedAt = now
	u.PasswordChangedAt = now
	m.users[userID] = u
	return nil
}

func (m *memStore) UpdateDefaultVisibility(_ context.Context, userID, visibility string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[userID]
	if !ok {
		return store.ErrNotFound
	}
	u.DefaultVisibility = visibility
	u.UpdatedAt = time.Now().UTC()
	m.users[userID] = u
	return nil
}

func (m *memStore) OwnerFileStats(_ context.Context, ownerID string, now time.Time, soonWithin time.Duration) (store.OwnerStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	soon := now.Add(soonWithin)
	var st store.OwnerStats
	for _, f := range m.files {
		if f.OwnerUserID == nil || *f.OwnerUserID != ownerID {
			continue
		}
		st.FileCount++
		st.TotalBytes += f.Size
		if f.Visibility == store.VisibilityPrivate {
			st.PrivateCount++
		} else {
			st.PublicCount++
		}
		if f.ExpiresAt.After(now) && !f.ExpiresAt.After(soon) {
			st.ExpiringSoonCount++
		}
	}
	return st, nil
}

func (m *memStore) UpdateVisibility(_ context.Context, id, visibility string, tokenHash *string, rotatedAt *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[id]
	if !ok {
		return store.ErrNotFound
	}
	f.Visibility = visibility
	if visibility == store.VisibilityPublic {
		f.AccessTokenHash = ""
		f.AccessTokenRotatedAt = nil
	} else if tokenHash != nil {
		f.AccessTokenHash = *tokenHash
		f.AccessTokenRotatedAt = rotatedAt
	}
	m.files[id] = f
	return nil
}

func (m *memStore) RotateAccessToken(_ context.Context, id, tokenHash string, rotatedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[id]
	if !ok || f.Visibility != store.VisibilityPrivate {
		return store.ErrNotFound
	}
	f.AccessTokenHash = tokenHash
	f.AccessTokenRotatedAt = &rotatedAt
	m.files[id] = f
	return nil
}

func (m *memStore) DeleteFile(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.files, id)
	return nil
}

func (m *memStore) DeleteExpiredFiles(_ context.Context, now time.Time, limit int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, f := range m.files {
		if n >= int64(limit) {
			break
		}
		if !f.ExpiresAt.After(now) {
			delete(m.files, id)
			n++
		}
	}
	return n, nil
}

func (m *memStore) ExtendExpiration(_ context.Context, id string, now time.Time, ext, capDur time.Duration) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[id]
	if !ok || !f.ExpiresAt.After(now) {
		return time.Time{}, store.ErrNotFound
	}
	next := f.ExpiresAt.Add(ext)
	capAt := now.Add(capDur)
	if next.After(capAt) {
		next = capAt
	}
	f.ExpiresAt = next
	m.files[id] = f
	return next, nil
}

func (m *memStore) Ping(context.Context) error { return nil }

type memCache struct {
	mu     sync.Mutex
	urls   map[string]string
	hits   int
	counts map[string]int64
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

func (c *memCache) Incr(_ context.Context, key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[string]int64{}
	}
	c.counts[key]++
	return c.counts[key], nil
}

func (c *memCache) Expire(context.Context, string, time.Duration) error { return nil }

func (c *memCache) Ping(context.Context) error { return nil }

func testOpts(publicBase string) Options {
	return Options{
		PublicBaseURL: publicBase,
		VisitorSalt:   "test-salt",
		WebOrigin:     "http://localhost:3000",
		CookieSecure:  false,
		Keys:          auth.DeriveKeys("test-app-secret-at-least-32-chars!!"),
	}
}

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
	srv := New(log, &memStore{files: map[string]store.File{}, chunks: map[string]store.Chunk{}}, ca, dc, testOpts(""))
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
	srv := New(log, &memStore{files: map[string]store.File{}, chunks: map[string]store.Chunk{}}, ca, dc, testOpts(public))
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

	// File list requires auth; anonymous uploads are not listed server-side.
	lr, err := http.Get(ts.URL + "/api/files")
	if err != nil {
		t.Fatal(err)
	}
	defer lr.Body.Close()
	if lr.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated list status = %d, want 401", lr.StatusCode)
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

	// ?download=1 also 302s for single-chunk (after retention extend).
	dl, err := noFollow.Get(ts.URL + "/f/" + up.FileID + "?download=1")
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	if dl.StatusCode != http.StatusFound {
		t.Fatalf("download status = %d, want 302", dl.StatusCode)
	}
	if loc := dl.Header.Get("Location"); !strings.Contains(loc, "/attachments/") {
		t.Fatalf("download Location = %q, want CDN attachment URL", loc)
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
