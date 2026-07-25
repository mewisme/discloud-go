// Package sqlite persists file metadata in a local SQLite database using the
// pure-Go modernc.org/sqlite driver, so the binary still builds with
// CGO_ENABLED=0.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite" // database/sql driver "sqlite"

	"github.com/mewisme/discloud-go/internal/store"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// connPragmas are applied per connection through the DSN. WAL plus a busy
// timeout keeps readers from tripping over the writer; foreign_keys enables the
// ON DELETE CASCADE the schema depends on.
var connPragmas = []string{
	"foreign_keys(1)",
	"journal_mode(WAL)",
	"busy_timeout(5000)",
	"synchronous(NORMAL)",
}

// fileColumns is the projection scanFileMeta expects.
const fileColumns = `id, name, size, chunk_size, created_at, owner_user_id, visibility, status,
	access_token_hash, access_token_rotated_at, expires_at,
	password_hash, max_downloads, download_count, share_mode, sha256`

type Store struct {
	db *sql.DB
}

// New opens the database at driverDSN (a modernc.org/sqlite DSN, typically
// "file:/path/discloud.db"). Missing PRAGMAs are added to the DSN.
func New(ctx context.Context, driverDSN string) (*Store, error) {
	db, err := sql.Open("sqlite", withPragmas(driverDSN))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite allows a single writer. One connection serializes every statement,
	// which removes write contention entirely and makes transactions the only
	// locking primitive this package needs.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	return &Store{db: db}, nil
}

// withPragmas appends any connPragmas the caller's DSN does not already set.
func withPragmas(dsn string) string {
	base, rawQuery, _ := strings.Cut(dsn, "?")
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		q = url.Values{}
	}
	set := make(map[string]bool, len(q["_pragma"]))
	for _, p := range q["_pragma"] {
		name, _, _ := strings.Cut(p, "(")
		set[strings.ToLower(strings.TrimSpace(name))] = true
	}
	for _, p := range connPragmas {
		name, _, _ := strings.Cut(p, "(")
		if !set[name] {
			q.Add("_pragma", p)
		}
	}
	return base + "?" + q.Encode()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Migrate applies embedded migrations that have not been applied yet,
// tracked by filename in schema_migrations.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries { // ReadDir returns names sorted
		var applied int
		if err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = ?)`, e.Name()).Scan(&applied); err != nil {
			return err
		}
		if applied == 1 {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", e.Name(), err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`,
			e.Name(), tsArg(time.Now())); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// EnsureBots upserts bot rows for token slots 0..count-1.
func (s *Store) EnsureBots(ctx context.Context, count int) error {
	now := tsArg(time.Now())
	for id := 0; id < count; id++ {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO bots (id, created_at) VALUES (?, ?) ON CONFLICT (id) DO NOTHING`, id, now); err != nil {
			return fmt.Errorf("ensure bot %d: %w", id, mapErr(err))
		}
	}
	return nil
}

// chunkInsertBatch is how many chunk rows go into one multi-row INSERT.
// 200 rows × 4 columns stays far below SQLITE_MAX_VARIABLE_NUMBER.
const chunkInsertBatch = 200

// CreateFile inserts the file row and its chunk rows in one transaction.
func (s *Store) CreateFile(ctx context.Context, f store.File) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	vis := f.Visibility
	if vis == "" {
		vis = store.VisibilityPublic
	}
	status := f.Status
	if status == "" {
		status = store.FileStatusReady
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO files (id, name, size, chunk_size, created_at, owner_user_id, visibility,
		                   status, access_token_hash, access_token_rotated_at, expires_at, sha256)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.Name, f.Size, f.ChunkSize, tsArg(f.CreatedAt), f.OwnerUserID, vis, status,
		nullText(f.AccessTokenHash), tsPtrArg(f.AccessTokenRotatedAt), tsArg(f.ExpiresAt),
		nullText(f.SHA256)); err != nil {
		return fmt.Errorf("insert file: %w", mapErr(err))
	}

	for start := 0; start < len(f.Parts); start += chunkInsertBatch {
		end := min(start+chunkInsertBatch, len(f.Parts))
		var b strings.Builder
		b.WriteString(`INSERT INTO chunks (file_id, idx, message_id, bot_id) VALUES `)
		args := make([]any, 0, (end-start)*4)
		for i := start; i < end; i++ {
			if i > start {
				b.WriteString(", ")
			}
			b.WriteString("(?, ?, ?, ?)")
			args = append(args, f.ID, i, f.Parts[i].MessageID, botArg(f.Parts[i].BotID))
		}
		if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
			return fmt.Errorf("insert chunks: %w", mapErr(err))
		}
	}
	return tx.Commit()
}

func (s *Store) GetFile(ctx context.Context, id string) (store.File, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+fileColumns+` FROM files WHERE id = ?`, id)
	f, err := scanFileMeta(row)
	if err != nil {
		return store.File{}, mapErr(err)
	}
	f.Parts, err = s.fileParts(ctx, id)
	if err != nil {
		return store.File{}, err
	}
	return f, nil
}

func (s *Store) fileParts(ctx context.Context, fileID string) ([]store.FilePart, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT message_id, bot_id FROM chunks WHERE file_id = ? ORDER BY idx`, fileID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var parts []store.FilePart
	for rows.Next() {
		var p store.FilePart
		var bot *int
		if err := rows.Scan(&p.MessageID, &bot); err != nil {
			return nil, err
		}
		p.BotID = botValue(bot)
		parts = append(parts, p)
	}
	return parts, rows.Err()
}

// FindFileByNameAndParts returns an unexpired file owned by ownerUserID with the
// same name and ordered Discord message_id list. ownerUserID must be set; other
// users' files are never matched.
func (s *Store) FindFileByNameAndParts(ctx context.Context, ownerUserID *string, name string, messageIDs []string, now time.Time) (store.File, error) {
	if ownerUserID == nil || *ownerUserID == "" || len(messageIDs) == 0 {
		return store.File{}, store.ErrNotFound
	}
	// SQLite has no array equality, so narrow to same-name, same-chunk-count
	// candidates in SQL and compare the ordered message IDs in Go.
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id
		FROM files f
		WHERE f.name = ?
		  AND f.owner_user_id = ?
		  AND f.expires_at > ?
		  AND (SELECT COUNT(*) FROM chunks c WHERE c.file_id = f.id) = ?
		ORDER BY f.created_at DESC, f.id DESC`,
		name, *ownerUserID, tsArg(now), len(messageIDs))
	if err != nil {
		return store.File{}, mapErr(err)
	}
	var candidates []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return store.File{}, err
		}
		candidates = append(candidates, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return store.File{}, err
	}

	for _, id := range candidates {
		parts, err := s.fileParts(ctx, id)
		if err != nil {
			return store.File{}, err
		}
		if len(parts) != len(messageIDs) {
			continue
		}
		match := true
		for i, p := range parts {
			if p.MessageID != messageIDs[i] {
				match = false
				break
			}
		}
		if match {
			return s.GetFile(ctx, id)
		}
	}
	return store.File{}, store.ErrNotFound
}

type scannable interface {
	Scan(dest ...any) error
}

func scanFileMeta(row scannable) (store.File, error) {
	var f store.File
	var owner, tokenHash, pwHash, sha *string
	var maxDL *int
	err := row.Scan(&f.ID, &f.Name, &f.Size, &f.ChunkSize, intoTS(&f.CreatedAt),
		&owner, &f.Visibility, &f.Status, &tokenHash, intoTSPtr(&f.AccessTokenRotatedAt),
		intoTS(&f.ExpiresAt), &pwHash, &maxDL, &f.DownloadCount, &f.ShareMode, &sha)
	if err != nil {
		return store.File{}, err
	}
	f.OwnerUserID = owner
	f.AccessTokenHash = deref(tokenHash)
	f.PasswordHash = deref(pwHash)
	f.MaxDownloads = maxDL
	f.SHA256 = deref(sha)
	if f.Status == "" {
		f.Status = store.FileStatusReady
	}
	if f.ShareMode == "" {
		f.ShareMode = store.ShareModeDownload
	}
	return f, nil
}

// UpdateFileStatus sets files.status (e.g. ready → reused on re-upload).
func (s *Store) UpdateFileStatus(ctx context.Context, id, status string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE files SET status = ? WHERE id = ?`, status, id))
}

func (s *Store) HasChunk(ctx context.Context, hash string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM chunk_store WHERE hash = ?)`, hash).Scan(&exists)
	return exists == 1, mapErr(err)
}

// PutChunk records an uploaded chunk; concurrent duplicate uploads are benign.
func (s *Store) PutChunk(ctx context.Context, c store.Chunk) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO chunk_store (hash, message_id, bot_id, size, created_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (hash) DO NOTHING`,
		c.Hash, c.MessageID, botArg(c.BotID), c.Size, tsArg(time.Now()))
	return mapErr(err)
}

// DeleteChunksByMessageID drops content-addressed rows for a Discord message.
func (s *Store) DeleteChunksByMessageID(ctx context.Context, messageID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chunk_store WHERE message_id = ?`, messageID)
	return mapErr(err)
}

// GetChunks resolves hashes to stored chunks; missing hashes are absent from
// the returned map.
func (s *Store) GetChunks(ctx context.Context, hashes []string) (map[string]store.Chunk, error) {
	out := make(map[string]store.Chunk, len(hashes))
	if len(hashes) == 0 {
		return out, nil
	}
	// Chunked IN (...) lists keep the bound-parameter count bounded.
	for start := 0; start < len(hashes); start += chunkInsertBatch {
		end := min(start+chunkInsertBatch, len(hashes))
		args := make([]any, 0, end-start)
		for _, h := range hashes[start:end] {
			args = append(args, h)
		}
		rows, err := s.db.QueryContext(ctx,
			`SELECT hash, message_id, bot_id, size FROM chunk_store WHERE hash IN (`+
				placeholders(len(args))+`)`, args...)
		if err != nil {
			return nil, mapErr(err)
		}
		for rows.Next() {
			var c store.Chunk
			var bot *int
			if err := rows.Scan(&c.Hash, &c.MessageID, &bot, &c.Size); err != nil {
				rows.Close()
				return nil, err
			}
			c.BotID = botValue(bot)
			out[c.Hash] = c
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) ListFilesByOwner(ctx context.Context, ownerID string, limit, offset int) ([]store.File, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+fileColumns+`
		FROM files
		WHERE owner_user_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?`, ownerID, limit, offset)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []store.File
	for rows.Next() {
		f, err := scanFileMeta(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
