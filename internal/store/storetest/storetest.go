// Package storetest is the backend-neutral contract for server.Store. Any
// backend (pgx, sqlite) can point Run at a freshly migrated, empty database and
// get the same behavioural guarantees checked.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mewisme/discloud-go/internal/ids"
	"github.com/mewisme/discloud-go/internal/server"
	"github.com/mewisme/discloud-go/internal/store"
)

// Run exercises the Store contract against s, which must be backed by a
// migrated database with no users yet (the first-admin rule needs that).
func Run(t *testing.T, s server.Store) {
	t.Helper()
	ctx := context.Background()
	if err := s.EnsureBots(ctx, 2); err != nil {
		t.Fatalf("EnsureBots: %v", err)
	}

	// FirstAdmin must run before any other user exists.
	t.Run("FirstAdmin", func(t *testing.T) { testFirstAdmin(t, ctx, s) })
	t.Run("Users", func(t *testing.T) { testUsers(t, ctx, s) })
	t.Run("Sessions", func(t *testing.T) { testSessions(t, ctx, s) })
	t.Run("Chunks", func(t *testing.T) { testChunks(t, ctx, s) })
	t.Run("Files", func(t *testing.T) { testFiles(t, ctx, s) })
	t.Run("Sharing", func(t *testing.T) { testSharing(t, ctx, s) })
	t.Run("UploadSessions", func(t *testing.T) { testUploadSessions(t, ctx, s) })
	t.Run("APITokens", func(t *testing.T) { testAPITokens(t, ctx, s) })
	t.Run("AdminOverview", func(t *testing.T) { testAdminOverview(t, ctx, s) })
}

// testFirstAdmin checks that concurrent signups elect exactly one admin.
func testFirstAdmin(t *testing.T, ctx context.Context, s server.Store) {
	const n = 16
	users := make([]store.User, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range users {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			users[i], errs[i] = s.CreateUser(ctx, ids.New(), fmt.Sprintf("first-%d", i), "hash")
		}(i)
	}
	wg.Wait()

	admins := 0
	for i, u := range users {
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

func testUsers(t *testing.T, ctx context.Context, s server.Store) {
	id, name := ids.New(), "alice-"+ids.New()
	u, err := s.CreateUser(ctx, id, name, "pwhash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Role != store.RoleUser {
		t.Fatalf("role = %q, want %q", u.Role, store.RoleUser)
	}
	if u.DefaultVisibility != store.VisibilityPublic {
		t.Fatalf("default visibility = %q", u.DefaultVisibility)
	}

	if _, err := s.CreateUser(ctx, ids.New(), name, "pwhash"); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate username error = %v, want ErrConflict", err)
	}

	got, err := s.GetUserByUsername(ctx, name)
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if got.ID != id || got.PasswordHash != "pwhash" {
		t.Fatalf("GetUserByUsername = %+v", got)
	}
	if !got.CreatedAt.Equal(u.CreatedAt) {
		t.Fatalf("created_at round-trip: got %v, want %v", got.CreatedAt, u.CreatedAt)
	}

	if _, err := s.GetUserByID(ctx, ids.New()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetUserByID(missing) = %v, want ErrNotFound", err)
	}

	if err := s.UpdateDefaultVisibility(ctx, id, store.VisibilityPrivate); err != nil {
		t.Fatalf("UpdateDefaultVisibility: %v", err)
	}
	if err := s.UpdatePasswordHash(ctx, id, "pwhash2"); err != nil {
		t.Fatalf("UpdatePasswordHash: %v", err)
	}
	got, err = s.GetUserByID(ctx, id)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.DefaultVisibility != store.VisibilityPrivate || got.PasswordHash != "pwhash2" {
		t.Fatalf("after updates = %+v", got)
	}
	if !got.PasswordChangedAt.After(u.PasswordChangedAt) && !got.PasswordChangedAt.Equal(u.PasswordChangedAt) {
		t.Fatalf("password_changed_at not bumped: %v", got.PasswordChangedAt)
	}

	if err := s.UpdatePasswordHash(ctx, ids.New(), "x"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("UpdatePasswordHash(missing) = %v, want ErrNotFound", err)
	}
}

func testSessions(t *testing.T, ctx context.Context, s server.Store) {
	userID := mustUser(t, ctx, s)
	now := time.Now().UTC()
	sess := store.Session{
		ID:        ids.New(),
		UserID:    userID,
		TokenHash: "sess-" + ids.New(),
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
		UserAgent: "go-test",
		IP:        "127.0.0.1",
	}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	u, err := s.GetUserBySessionHash(ctx, sess.TokenHash, now)
	if err != nil {
		t.Fatalf("GetUserBySessionHash: %v", err)
	}
	if u.ID != userID {
		t.Fatalf("session user = %q, want %q", u.ID, userID)
	}

	got, err := s.GetSessionByTokenHash(ctx, sess.TokenHash, now)
	if err != nil {
		t.Fatalf("GetSessionByTokenHash: %v", err)
	}
	if !got.ExpiresAt.Equal(sess.ExpiresAt) || !got.LastSeenAt.Equal(sess.CreatedAt) {
		t.Fatalf("session timestamps = %+v", got)
	}
	if got.UserAgent != "go-test" || got.IP != "127.0.0.1" {
		t.Fatalf("session metadata = %+v", got)
	}

	// Expired sessions are invisible.
	if _, err := s.GetSessionByTokenHash(ctx, sess.TokenHash, now.Add(2*time.Hour)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expired session = %v, want ErrNotFound", err)
	}

	later := now.Add(time.Minute)
	if err := s.TouchSession(ctx, sess.TokenHash, "10.0.0.1", "curl", later); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	got, err = s.GetSessionByTokenHash(ctx, sess.TokenHash, later)
	if err != nil {
		t.Fatalf("GetSessionByTokenHash after touch: %v", err)
	}
	if !got.LastSeenAt.Equal(later) || got.IP != "10.0.0.1" || got.UserAgent != "curl" {
		t.Fatalf("touched session = %+v", got)
	}

	if err := s.DeleteSession(ctx, sess.TokenHash); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := s.GetSessionByTokenHash(ctx, sess.TokenHash, now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted session = %v, want ErrNotFound", err)
	}

	sess.ID, sess.TokenHash = ids.New(), "sess2-"+ids.New()
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession 2: %v", err)
	}
	if err := s.DeleteSessionsByUserID(ctx, userID); err != nil {
		t.Fatalf("DeleteSessionsByUserID: %v", err)
	}
	if _, err := s.GetSessionByTokenHash(ctx, sess.TokenHash, now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("session after bulk delete = %v, want ErrNotFound", err)
	}
}

func testChunks(t *testing.T, ctx context.Context, s server.Store) {
	msgID := "msg-" + ids.New()
	a := store.Chunk{Hash: "h-" + ids.New(), MessageID: msgID, BotID: 1, Size: 1024}
	b := store.Chunk{Hash: "h-" + ids.New(), MessageID: "msg-" + ids.New(), BotID: -1, Size: 2048}

	for _, c := range []store.Chunk{a, b} {
		if err := s.PutChunk(ctx, c); err != nil {
			t.Fatalf("PutChunk: %v", err)
		}
	}
	// Re-registering the same hash is benign.
	if err := s.PutChunk(ctx, a); err != nil {
		t.Fatalf("PutChunk duplicate: %v", err)
	}

	ok, err := s.HasChunk(ctx, a.Hash)
	if err != nil || !ok {
		t.Fatalf("HasChunk(%q) = %v, %v", a.Hash, ok, err)
	}
	ok, err = s.HasChunk(ctx, "nope")
	if err != nil || ok {
		t.Fatalf("HasChunk(missing) = %v, %v", ok, err)
	}

	got, err := s.GetChunks(ctx, []string{a.Hash, b.Hash, "missing"})
	if err != nil {
		t.Fatalf("GetChunks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetChunks returned %d chunks, want 2", len(got))
	}
	if got[a.Hash] != a {
		t.Fatalf("GetChunks[a] = %+v, want %+v", got[a.Hash], a)
	}
	if got[b.Hash].BotID != -1 {
		t.Fatalf("unknown bot slot = %d, want -1", got[b.Hash].BotID)
	}

	if err := s.DeleteChunksByMessageID(ctx, msgID); err != nil {
		t.Fatalf("DeleteChunksByMessageID: %v", err)
	}
	if ok, _ := s.HasChunk(ctx, a.Hash); ok {
		t.Fatal("chunk still present after DeleteChunksByMessageID")
	}
}

func testFiles(t *testing.T, ctx context.Context, s server.Store) {
	owner := mustUser(t, ctx, s)
	now := time.Now().UTC()
	f := store.File{
		ID:          ids.New(),
		Name:        "report.pdf",
		Size:        3000,
		ChunkSize:   1000,
		CreatedAt:   now,
		OwnerUserID: &owner,
		Visibility:  store.VisibilityPublic,
		ExpiresAt:   now.Add(24 * time.Hour),
		SHA256:      "sha-" + ids.New(),
		Parts: []store.FilePart{
			{MessageID: "m1", BotID: 0},
			{MessageID: "m2", BotID: 1},
			{MessageID: "m3", BotID: -1},
		},
	}
	if err := s.CreateFile(ctx, f); err != nil {
		t.Fatalf("CreateFile: %v", err)
	}

	got, err := s.GetFile(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if got.Name != f.Name || got.Size != f.Size || got.ChunkSize != f.ChunkSize {
		t.Fatalf("GetFile meta = %+v", got)
	}
	if got.OwnerUserID == nil || *got.OwnerUserID != owner {
		t.Fatalf("owner = %v, want %q", got.OwnerUserID, owner)
	}
	if !got.CreatedAt.Equal(f.CreatedAt) || !got.ExpiresAt.Equal(f.ExpiresAt) {
		t.Fatalf("timestamps = %v / %v, want %v / %v", got.CreatedAt, got.ExpiresAt, f.CreatedAt, f.ExpiresAt)
	}
	if got.Status != store.FileStatusReady || got.ShareMode != store.ShareModeDownload {
		t.Fatalf("defaults = %q / %q", got.Status, got.ShareMode)
	}
	if got.SHA256 != f.SHA256 {
		t.Fatalf("sha256 = %q, want %q", got.SHA256, f.SHA256)
	}
	if len(got.Parts) != 3 || got.Parts[0] != f.Parts[0] || got.Parts[2].BotID != -1 {
		t.Fatalf("parts = %+v", got.Parts)
	}

	if _, err := s.GetFile(ctx, ids.New()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetFile(missing) = %v, want ErrNotFound", err)
	}

	if err := s.UpdateFileStatus(ctx, f.ID, store.FileStatusReused); err != nil {
		t.Fatalf("UpdateFileStatus: %v", err)
	}
	if got, _ = s.GetFile(ctx, f.ID); got.Status != store.FileStatusReused {
		t.Fatalf("status = %q, want %q", got.Status, store.FileStatusReused)
	}

	// Dedup lookup: same owner + name + ordered message IDs.
	dup, err := s.FindFileByNameAndParts(ctx, &owner, f.Name, []string{"m1", "m2", "m3"}, now)
	if err != nil {
		t.Fatalf("FindFileByNameAndParts: %v", err)
	}
	if dup.ID != f.ID {
		t.Fatalf("dedup matched %q, want %q", dup.ID, f.ID)
	}
	// Order matters, and so does the part count.
	for _, msgs := range [][]string{{"m1", "m3", "m2"}, {"m1", "m2"}, {"m1", "m2", "m3", "m4"}} {
		if _, err := s.FindFileByNameAndParts(ctx, &owner, f.Name, msgs, now); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("FindFileByNameAndParts(%v) = %v, want ErrNotFound", msgs, err)
		}
	}
	// Another owner never matches.
	other := mustUser(t, ctx, s)
	if _, err := s.FindFileByNameAndParts(ctx, &other, f.Name, []string{"m1", "m2", "m3"}, now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-owner dedup = %v, want ErrNotFound", err)
	}

	list, err := s.ListFilesByOwner(ctx, owner, 10, 0)
	if err != nil {
		t.Fatalf("ListFilesByOwner: %v", err)
	}
	if len(list) != 1 || list[0].ID != f.ID {
		t.Fatalf("ListFilesByOwner = %+v", list)
	}

	stats, err := s.OwnerFileStats(ctx, owner, now, 48*time.Hour)
	if err != nil {
		t.Fatalf("OwnerFileStats: %v", err)
	}
	if stats.FileCount != 1 || stats.TotalBytes != 3000 || stats.PublicCount != 1 ||
		stats.PrivateCount != 0 || stats.ExpiringSoonCount != 1 {
		t.Fatalf("OwnerFileStats = %+v", stats)
	}

	// Visibility + access token.
	hash := "tok-" + ids.New()
	rotated := now.Add(time.Minute)
	if err := s.UpdateVisibility(ctx, f.ID, store.VisibilityPrivate, &hash, &rotated); err != nil {
		t.Fatalf("UpdateVisibility: %v", err)
	}
	got, _ = s.GetFile(ctx, f.ID)
	if got.Visibility != store.VisibilityPrivate || got.AccessTokenHash != hash ||
		got.AccessTokenRotatedAt == nil || !got.AccessTokenRotatedAt.Equal(rotated) {
		t.Fatalf("after UpdateVisibility = %+v", got)
	}
	if err := s.RotateAccessToken(ctx, f.ID, "tok2", now); err != nil {
		t.Fatalf("RotateAccessToken: %v", err)
	}
	if got, _ = s.GetFile(ctx, f.ID); got.AccessTokenHash != "tok2" {
		t.Fatalf("rotated hash = %q", got.AccessTokenHash)
	}
	if err := s.UpdateVisibility(ctx, f.ID, store.VisibilityPublic, nil, nil); err != nil {
		t.Fatalf("UpdateVisibility public: %v", err)
	}
	if got, _ = s.GetFile(ctx, f.ID); got.AccessTokenHash != "" || got.AccessTokenRotatedAt != nil {
		t.Fatalf("public file kept its token: %+v", got)
	}
	// A public file cannot be rotated.
	if err := s.RotateAccessToken(ctx, f.ID, "tok3", now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("RotateAccessToken(public) = %v, want ErrNotFound", err)
	}

	// Expiry is extended by ext, then clamped to now+cap.
	exp, err := s.ExtendExpiration(ctx, f.ID, now, time.Hour, 100*time.Hour)
	if err != nil {
		t.Fatalf("ExtendExpiration: %v", err)
	}
	if want := f.ExpiresAt.Add(time.Hour); !exp.Equal(want) {
		t.Fatalf("extended to %v, want %v", exp, want)
	}
	exp, err = s.ExtendExpiration(ctx, f.ID, now, 999*time.Hour, 30*time.Hour)
	if err != nil {
		t.Fatalf("ExtendExpiration capped: %v", err)
	}
	if want := now.Add(30 * time.Hour); !exp.Equal(want) {
		t.Fatalf("capped expiry = %v, want %v", exp, want)
	}

	// Access counters and unique visitors.
	for _, e := range []store.Event{
		{FileID: f.ID, Kind: store.EventView, VisitorHash: "v1"},
		{FileID: f.ID, Kind: store.EventDownload, Bytes: 3000, VisitorHash: "v1"},
		{FileID: f.ID, Kind: store.EventRange, Bytes: 500, VisitorHash: "v2"},
	} {
		if err := s.RecordEvent(ctx, e); err != nil {
			t.Fatalf("RecordEvent %s: %v", e.Kind, err)
		}
	}
	if err := s.RecordEvent(ctx, store.Event{FileID: ids.New(), Kind: store.EventView}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("RecordEvent(missing file) = %v, want ErrNotFound", err)
	}
	insp, err := s.GetFileInspect(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFileInspect: %v", err)
	}
	if insp.Views != 1 || insp.Downloads != 1 || insp.Ranges != 1 ||
		insp.BytesServed != 3500 || insp.UniqueVisitors != 2 || insp.ChunkCount != 3 {
		t.Fatalf("inspect = %+v", insp)
	}
	if insp.LastAccessAt == nil {
		t.Fatal("inspect.LastAccessAt is nil")
	}

	if err := s.DeleteFile(ctx, f.ID); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if err := s.DeleteFile(ctx, f.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteFile(missing) = %v, want ErrNotFound", err)
	}

	// Expired files are swept.
	stale := store.File{
		ID: ids.New(), Name: "old.bin", Size: 1, ChunkSize: 1, CreatedAt: now.Add(-48 * time.Hour),
		OwnerUserID: &owner, ExpiresAt: now.Add(-time.Hour),
		Parts: []store.FilePart{{MessageID: "m9", BotID: -1}},
	}
	if err := s.CreateFile(ctx, stale); err != nil {
		t.Fatalf("CreateFile(stale): %v", err)
	}
	n, err := s.DeleteExpiredFiles(ctx, now, 10)
	if err != nil {
		t.Fatalf("DeleteExpiredFiles: %v", err)
	}
	if n < 1 {
		t.Fatalf("DeleteExpiredFiles deleted %d rows, want >= 1", n)
	}
	if _, err := s.GetFile(ctx, stale.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("stale file survived: %v", err)
	}
}

func testSharing(t *testing.T, ctx context.Context, s server.Store) {
	owner := mustUser(t, ctx, s)
	now := time.Now().UTC()
	f := store.File{
		ID: ids.New(), Name: "share.zip", Size: 10, ChunkSize: 10, CreatedAt: now,
		OwnerUserID: &owner, ExpiresAt: now.Add(time.Hour),
		Parts: []store.FilePart{{MessageID: "s1", BotID: 0}},
	}
	if err := s.CreateFile(ctx, f); err != nil {
		t.Fatalf("CreateFile: %v", err)
	}

	pw, mode, maxDL := "pw-hash", store.ShareModeView, 1
	newExp := now.Add(2 * time.Hour)
	if err := s.UpdateFileShare(ctx, f.ID, store.FileSharePatch{
		PasswordHash: &pw, ExpiresAt: &newExp, MaxDownloads: &maxDL, ShareMode: &mode,
	}); err != nil {
		t.Fatalf("UpdateFileShare: %v", err)
	}
	got, _ := s.GetFile(ctx, f.ID)
	if got.PasswordHash != pw || got.ShareMode != mode || got.MaxDownloads == nil ||
		*got.MaxDownloads != 1 || !got.ExpiresAt.Equal(newExp) {
		t.Fatalf("after share patch = %+v", got)
	}

	if err := s.IncrementDownloadCount(ctx, f.ID); err != nil {
		t.Fatalf("IncrementDownloadCount: %v", err)
	}
	if err := s.IncrementDownloadCount(ctx, f.ID); !errors.Is(err, store.ErrDownloadLimit) {
		t.Fatalf("IncrementDownloadCount over cap = %v, want ErrDownloadLimit", err)
	}
	if err := s.IncrementDownloadCount(ctx, ids.New()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("IncrementDownloadCount(missing) = %v, want ErrNotFound", err)
	}

	// Clearing leaves the untouched fields alone.
	if err := s.UpdateFileShare(ctx, f.ID, store.FileSharePatch{
		ClearPassword: true, ClearMaxDownloads: true,
	}); err != nil {
		t.Fatalf("UpdateFileShare clear: %v", err)
	}
	got, _ = s.GetFile(ctx, f.ID)
	if got.PasswordHash != "" || got.MaxDownloads != nil || got.ShareMode != mode {
		t.Fatalf("after clear = %+v", got)
	}
	if err := s.IncrementDownloadCount(ctx, f.ID); err != nil {
		t.Fatalf("IncrementDownloadCount uncapped: %v", err)
	}

	if err := s.RecordEvent(ctx, store.Event{
		FileID: f.ID, Kind: store.EventDownload, Bytes: f.Size, VisitorHash: "v1",
	}); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}

	if err := s.RevokeFile(ctx, f.ID, now); err != nil {
		t.Fatalf("RevokeFile: %v", err)
	}
	got, _ = s.GetFile(ctx, f.ID)
	if !got.ExpiresAt.Equal(now) || got.PasswordHash != "" {
		t.Fatalf("after revoke = %+v", got)
	}
}

func testUploadSessions(t *testing.T, ctx context.Context, s server.Store) {
	owner := mustUser(t, ctx, s)
	now := time.Now().UTC()
	u := store.UploadSession{
		ID: ids.New(), OwnerUserID: &owner, ResumeTokenHash: "resume-" + ids.New(),
		FileName: "big.iso", FileSize: 2048, ChunkSize: 1024, ChunkCount: 2,
		Status: store.UploadPending, ClientFingerprint: "ip:1.2.3.4",
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := s.CreateUploadSession(ctx, u); err != nil {
		t.Fatalf("CreateUploadSession: %v", err)
	}

	got, err := s.GetUploadSession(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUploadSession: %v", err)
	}
	if got.FileName != u.FileName || got.ChunkCount != 2 || got.Status != store.UploadPending ||
		got.FileID != nil || len(got.Parts) != 0 || !got.ExpiresAt.Equal(u.ExpiresAt) {
		t.Fatalf("GetUploadSession = %+v", got)
	}
	if got.OwnerUserID == nil || *got.OwnerUserID != owner {
		t.Fatalf("upload owner = %v", got.OwnerUserID)
	}

	n, err := s.CountOpenUploadSessionsByOwner(ctx, owner)
	if err != nil || n != 1 {
		t.Fatalf("CountOpenUploadSessionsByOwner = %d, %v", n, err)
	}
	bytes, err := s.SumOpenUploadBytesByOwner(ctx, owner)
	if err != nil || bytes != 2048 {
		t.Fatalf("SumOpenUploadBytesByOwner = %d, %v", bytes, err)
	}

	// Parts: register, idempotent re-register, conflicting re-register.
	if err := s.RegisterUploadPart(ctx, u.ID, 0, "hash0", now); err != nil {
		t.Fatalf("RegisterUploadPart: %v", err)
	}
	if err := s.RegisterUploadPart(ctx, u.ID, 0, "hash0", now); err != nil {
		t.Fatalf("RegisterUploadPart idempotent: %v", err)
	}
	if err := s.RegisterUploadPart(ctx, u.ID, 0, "other", now); !errors.Is(err, store.ErrPartConflict) {
		t.Fatalf("RegisterUploadPart conflict = %v, want ErrPartConflict", err)
	}
	if err := s.RegisterUploadPart(ctx, u.ID, 5, "hash5", now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("RegisterUploadPart out of range = %v, want ErrNotFound", err)
	}
	if err := s.RegisterUploadPart(ctx, u.ID, 1, "hash1", now); err != nil {
		t.Fatalf("RegisterUploadPart 1: %v", err)
	}
	got, _ = s.GetUploadSession(ctx, u.ID)
	if got.Status != store.UploadUploading || got.Parts[0] != "hash0" || got.Parts[1] != "hash1" {
		t.Fatalf("after parts = %+v", got)
	}

	// Complete: begin → abort → begin → finish.
	begun, err := s.BeginUploadComplete(ctx, u.ID, now)
	if err != nil {
		t.Fatalf("BeginUploadComplete: %v", err)
	}
	if begun.Status != store.UploadCompleting || len(begun.Parts) != 2 {
		t.Fatalf("begun = %+v", begun)
	}
	if err := s.AbortUploadComplete(ctx, u.ID, now); err != nil {
		t.Fatalf("AbortUploadComplete: %v", err)
	}
	if got, _ = s.GetUploadSession(ctx, u.ID); got.Status != store.UploadUploading {
		t.Fatalf("after abort status = %q", got.Status)
	}
	if _, err := s.BeginUploadComplete(ctx, u.ID, now); err != nil {
		t.Fatalf("BeginUploadComplete 2: %v", err)
	}

	fileID := ids.New()
	if err := s.CreateFile(ctx, store.File{
		ID: fileID, Name: u.FileName, Size: u.FileSize, ChunkSize: int64(u.ChunkSize),
		CreatedAt: now, OwnerUserID: &owner, ExpiresAt: now.Add(time.Hour),
		Parts: []store.FilePart{{MessageID: "u1", BotID: 0}, {MessageID: "u2", BotID: 1}},
	}); err != nil {
		t.Fatalf("CreateFile for upload: %v", err)
	}
	if err := s.FinishUploadComplete(ctx, u.ID, fileID, now); err != nil {
		t.Fatalf("FinishUploadComplete: %v", err)
	}
	if err := s.FinishUploadComplete(ctx, u.ID, fileID, now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("FinishUploadComplete twice = %v, want ErrNotFound", err)
	}
	// Completing an already-completed session is idempotent.
	done, err := s.BeginUploadComplete(ctx, u.ID, now)
	if err != nil {
		t.Fatalf("BeginUploadComplete(completed): %v", err)
	}
	if done.Status != store.UploadCompleted || done.FileID == nil || *done.FileID != fileID {
		t.Fatalf("completed session = %+v", done)
	}
	if n, _ := s.CountOpenUploadSessionsByOwner(ctx, owner); n != 0 {
		t.Fatalf("open sessions after completion = %d, want 0", n)
	}

	// Cancel.
	c := u
	c.ID, c.ResumeTokenHash, c.Status = ids.New(), "resume-"+ids.New(), store.UploadPending
	if err := s.CreateUploadSession(ctx, c); err != nil {
		t.Fatalf("CreateUploadSession(cancel): %v", err)
	}
	if err := s.CancelUploadSession(ctx, c.ID, now); err != nil {
		t.Fatalf("CancelUploadSession: %v", err)
	}
	if err := s.CancelUploadSession(ctx, c.ID, now); !errors.Is(err, store.ErrUploadNotActive) {
		t.Fatalf("CancelUploadSession twice = %v, want ErrUploadNotActive", err)
	}
	if err := s.CancelUploadSession(ctx, ids.New(), now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("CancelUploadSession(missing) = %v, want ErrNotFound", err)
	}

	// Expiry sweep.
	e := u
	e.ID, e.ResumeTokenHash, e.Status = ids.New(), "resume-"+ids.New(), store.UploadPending
	e.ExpiresAt = now.Add(-time.Minute)
	if err := s.CreateUploadSession(ctx, e); err != nil {
		t.Fatalf("CreateUploadSession(expired): %v", err)
	}
	swept, err := s.ExpireUploadSessions(ctx, now, 10)
	if err != nil {
		t.Fatalf("ExpireUploadSessions: %v", err)
	}
	if swept < 1 {
		t.Fatalf("ExpireUploadSessions swept %d, want >= 1", swept)
	}
	if got, _ = s.GetUploadSession(ctx, e.ID); got.Status != store.UploadExpired {
		t.Fatalf("expired session status = %q", got.Status)
	}
	// An expired session is no longer completable.
	if _, err := s.BeginUploadComplete(ctx, e.ID, now); !errors.Is(err, store.ErrUploadNotActive) {
		t.Fatalf("BeginUploadComplete(expired) = %v, want ErrUploadNotActive", err)
	}

	if _, err := s.GetUploadSession(ctx, ids.New()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetUploadSession(missing) = %v, want ErrNotFound", err)
	}

	// Anonymous sessions are counted by fingerprint prefix.
	anon := u
	anon.ID, anon.ResumeTokenHash, anon.OwnerUserID = ids.New(), "resume-"+ids.New(), nil
	anon.Status, anon.ClientFingerprint = store.UploadPending, "anon-"+ids.New()+":agent"
	if err := s.CreateUploadSession(ctx, anon); err != nil {
		t.Fatalf("CreateUploadSession(anon): %v", err)
	}
	n, err = s.CountOpenUploadSessionsAnon(ctx, anon.ClientFingerprint[:12])
	if err != nil || n != 1 {
		t.Fatalf("CountOpenUploadSessionsAnon = %d, %v", n, err)
	}
}

func testAPITokens(t *testing.T, ctx context.Context, s server.Store) {
	userID := mustUser(t, ctx, s)
	now := time.Now().UTC()
	expires := now.Add(30 * 24 * time.Hour)
	tok := store.APIToken{
		ID: ids.New(), UserID: userID, Name: "ci",
		TokenHash: "pat-" + ids.New(),
		Scopes:    []string{store.ScopeUpload, store.ScopeRead},
		ExpiresAt: &expires, CreatedAt: now,
	}
	if err := s.CreateAPIToken(ctx, tok); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}

	got, err := s.GetAPITokenByHash(ctx, tok.TokenHash)
	if err != nil {
		t.Fatalf("GetAPITokenByHash: %v", err)
	}
	if got.ID != tok.ID || got.UserID != userID || got.Name != "ci" {
		t.Fatalf("token = %+v", got)
	}
	if len(got.Scopes) != 2 || got.Scopes[0] != store.ScopeUpload || got.Scopes[1] != store.ScopeRead {
		t.Fatalf("scopes = %v, want %v", got.Scopes, tok.Scopes)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("expires_at = %v, want %v", got.ExpiresAt, expires)
	}
	if got.RevokedAt != nil || got.LastUsedAt != nil {
		t.Fatalf("fresh token already revoked/used: %+v", got)
	}
	if _, err := s.GetAPITokenByHash(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetAPITokenByHash(missing) = %v, want ErrNotFound", err)
	}

	used := now.Add(time.Minute)
	if err := s.TouchAPITokenLastUsed(ctx, tok.ID, used); err != nil {
		t.Fatalf("TouchAPITokenLastUsed: %v", err)
	}
	if got, _ = s.GetAPITokenByHash(ctx, tok.TokenHash); got.LastUsedAt == nil || !got.LastUsedAt.Equal(used) {
		t.Fatalf("last_used_at = %v, want %v", got.LastUsedAt, used)
	}

	// A second token with no expiry and no scopes.
	tok2 := store.APIToken{
		ID: ids.New(), UserID: userID, Name: "laptop",
		TokenHash: "pat-" + ids.New(), CreatedAt: now.Add(time.Second),
	}
	if err := s.CreateAPIToken(ctx, tok2); err != nil {
		t.Fatalf("CreateAPIToken 2: %v", err)
	}
	got, _ = s.GetAPITokenByHash(ctx, tok2.TokenHash)
	if got.ExpiresAt != nil || len(got.Scopes) != 0 {
		t.Fatalf("token 2 = %+v", got)
	}

	list, err := s.ListAPITokensByUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListAPITokensByUser: %v", err)
	}
	if len(list) != 2 || list[0].ID != tok2.ID {
		t.Fatalf("token list = %+v, want newest first", list)
	}
	if n, err := s.CountAPITokensByUser(ctx, userID); err != nil || n != 2 {
		t.Fatalf("CountAPITokensByUser = %d, %v", n, err)
	}

	if err := s.RevokeAPIToken(ctx, tok.ID, userID, now); err != nil {
		t.Fatalf("RevokeAPIToken: %v", err)
	}
	if err := s.RevokeAPIToken(ctx, tok.ID, userID, now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("RevokeAPIToken twice = %v, want ErrNotFound", err)
	}
	if got, _ = s.GetAPITokenByHash(ctx, tok.TokenHash); got.RevokedAt == nil {
		t.Fatal("revoked token has no revoked_at")
	}
	if n, err := s.CountAPITokensByUser(ctx, userID); err != nil || n != 1 {
		t.Fatalf("CountAPITokensByUser after revoke = %d, %v", n, err)
	}

	if err := s.RevokeAPITokensByUser(ctx, userID, now); err != nil {
		t.Fatalf("RevokeAPITokensByUser: %v", err)
	}
	if n, err := s.CountAPITokensByUser(ctx, userID); err != nil || n != 0 {
		t.Fatalf("CountAPITokensByUser after bulk revoke = %d, %v", n, err)
	}
}

func testAdminOverview(t *testing.T, ctx context.Context, s server.Store) {
	o, err := s.AdminOverview(ctx, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("AdminOverview: %v", err)
	}
	if o.UserCount == 0 || o.AdminCount != 1 {
		t.Fatalf("users = %d, admins = %d, want admins = 1", o.UserCount, o.AdminCount)
	}
	if o.FileCount == 0 || o.TotalBytes == 0 {
		t.Fatalf("files = %d, bytes = %d, want both > 0", o.FileCount, o.TotalBytes)
	}
	if o.Completed24h == 0 || o.Cancelled24h == 0 || o.Expired24h == 0 {
		t.Fatalf("upload counters = %+v, want all > 0", o)
	}
	if o.Downloads == 0 || o.BytesServed == 0 {
		t.Fatalf("traffic = %d downloads, %d bytes", o.Downloads, o.BytesServed)
	}
}

func mustUser(t *testing.T, ctx context.Context, s server.Store) string {
	t.Helper()
	id := ids.New()
	if _, err := s.CreateUser(ctx, id, "u-"+id, "hash"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return id
}
