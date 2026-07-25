// Package pgx persists file metadata in PostgreSQL.
package pgx

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mewisme/discloud-go/internal/store"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Migrate applies embedded migrations that have not been applied yet,
// tracked by filename in schema_migrations.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries { // ReadDir returns names sorted
		var applied bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = $1)`, e.Name()).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		sql, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", e.Name(), err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, e.Name()); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// EnsureBots upserts bot rows for token slots 0..count-1.
func (s *Store) EnsureBots(ctx context.Context, count int) error {
	for id := 0; id < count; id++ {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO bots (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`, id); err != nil {
			return fmt.Errorf("ensure bot %d: %w", id, err)
		}
	}
	return nil
}

// CreateFile inserts the file row and its chunk rows in one transaction.
func (s *Store) CreateFile(ctx context.Context, f store.File) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	vis := f.Visibility
	if vis == "" {
		vis = store.VisibilityPublic
	}
	status := f.Status
	if status == "" {
		status = store.FileStatusReady
	}
	var tokenHash any
	if f.AccessTokenHash != "" {
		tokenHash = f.AccessTokenHash
	}
	var rotated any
	if f.AccessTokenRotatedAt != nil {
		rotated = *f.AccessTokenRotatedAt
	}
	var sha any
	if f.SHA256 != "" {
		sha = f.SHA256
	}
	if _, err := tx.Exec(ctx, `
					INSERT INTO files (id, name, size, chunk_size, created_at, owner_user_id, visibility,
					                   status, access_token_hash, access_token_rotated_at, expires_at, sha256)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		f.ID, f.Name, f.Size, f.ChunkSize, f.CreatedAt, f.OwnerUserID, vis, status,
		tokenHash, rotated, f.ExpiresAt, sha); err != nil {
		return fmt.Errorf("insert file: %w", err)
	}
	rows := make([][]any, len(f.Parts))
	for i, p := range f.Parts {
		var bot any
		if p.BotID >= 0 {
			bot = p.BotID
		}
		rows[i] = []any{f.ID, i, p.MessageID, bot}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"chunks"},
		[]string{"file_id", "idx", "message_id", "bot_id"}, pgx.CopyFromRows(rows)); err != nil {
		return fmt.Errorf("insert chunks: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Store) GetFile(ctx context.Context, id string) (store.File, error) {
	row := s.pool.QueryRow(ctx, `
				SELECT id, name, size, chunk_size, created_at, owner_user_id, visibility, status,
				       access_token_hash, access_token_rotated_at, expires_at,
				       password_hash, max_downloads, download_count, share_mode, sha256
				FROM files WHERE id = $1`, id)
	f, err := scanFileMeta(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.File{}, store.ErrNotFound
	}
	if err != nil {
		return store.File{}, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT message_id, bot_id FROM chunks WHERE file_id = $1 ORDER BY idx`, id)
	if err != nil {
		return store.File{}, err
	}
	f.Parts, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.FilePart, error) {
		var p store.FilePart
		var bot *int
		if err := row.Scan(&p.MessageID, &bot); err != nil {
			return store.FilePart{}, err
		}
		if bot != nil {
			p.BotID = *bot
		} else {
			p.BotID = -1
		}
		return p, nil
	})
	return f, err
}

// FindFileByNameAndParts returns an unexpired file owned by ownerUserID with the
// same name and ordered Discord message_id list. ownerUserID must be set; other
// users' files are never matched.
func (s *Store) FindFileByNameAndParts(ctx context.Context, ownerUserID *string, name string, messageIDs []string, now time.Time) (store.File, error) {
	if ownerUserID == nil || *ownerUserID == "" || len(messageIDs) == 0 {
		return store.File{}, store.ErrNotFound
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT f.id
		FROM files f
		WHERE f.name = $1
		  AND f.owner_user_id = $2::uuid
		  AND f.expires_at > $3
		  AND (
		    SELECT array_agg(c.message_id ORDER BY c.idx)
		    FROM chunks c WHERE c.file_id = f.id
		  ) = $4::text[]
		ORDER BY f.created_at DESC, f.id DESC
		LIMIT 1`, name, *ownerUserID, now, messageIDs).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.File{}, store.ErrNotFound
	}
	if err != nil {
		return store.File{}, err
	}
	return s.GetFile(ctx, store.UuidHex(id))
}

type scannable interface {
	Scan(dest ...any) error
}

func scanFileMeta(row scannable) (store.File, error) {
	var f store.File
	var owner *string
	var tokenHash *string
	var rotated *time.Time
	var pwHash *string
	var maxDL *int
	var sha *string
	err := row.Scan(&f.ID, &f.Name, &f.Size, &f.ChunkSize, &f.CreatedAt,
		&owner, &f.Visibility, &f.Status, &tokenHash, &rotated, &f.ExpiresAt,
		&pwHash, &maxDL, &f.DownloadCount, &f.ShareMode, &sha)
	if err != nil {
		return store.File{}, err
	}
	f.ID = store.UuidHex(f.ID)
	if owner != nil {
		h := store.UuidHex(*owner)
		f.OwnerUserID = &h
	}
	if tokenHash != nil {
		f.AccessTokenHash = *tokenHash
	}
	f.AccessTokenRotatedAt = rotated
	if pwHash != nil {
		f.PasswordHash = *pwHash
	}
	f.MaxDownloads = maxDL
	if sha != nil {
		f.SHA256 = *sha
	}
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
	tag, err := s.pool.Exec(ctx, `UPDATE files SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) HasChunk(ctx context.Context, hash string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM chunk_store WHERE hash = $1)`, hash).Scan(&exists)
	return exists, err
}

// PutChunk records an uploaded chunk; concurrent duplicate uploads are benign.
func (s *Store) PutChunk(ctx context.Context, c store.Chunk) error {
	var bot any
	if c.BotID >= 0 {
		bot = c.BotID
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO chunk_store (hash, message_id, bot_id, size) VALUES ($1, $2, $3, $4) ON CONFLICT (hash) DO NOTHING`,
		c.Hash, c.MessageID, bot, c.Size)
	return err
}

// DeleteChunksByMessageID drops content-addressed rows for a Discord message.
func (s *Store) DeleteChunksByMessageID(ctx context.Context, messageID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM chunk_store WHERE message_id = $1`, messageID)
	return err
}

// GetChunks resolves hashes to stored chunks; missing hashes are absent from
// the returned map.
func (s *Store) GetChunks(ctx context.Context, hashes []string) (map[string]store.Chunk, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT hash, message_id, bot_id, size FROM chunk_store WHERE hash = ANY($1)`, hashes)
	if err != nil {
		return nil, err
	}
	chunks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.Chunk, error) {
		var c store.Chunk
		var bot *int
		err := row.Scan(&c.Hash, &c.MessageID, &bot, &c.Size)
		if err != nil {
			return store.Chunk{}, err
		}
		if bot != nil {
			c.BotID = *bot
		} else {
			c.BotID = -1
		}
		return c, nil
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]store.Chunk, len(chunks))
	for _, c := range chunks {
		out[c.Hash] = c
	}
	return out, nil
}

func (s *Store) ListFilesByOwner(ctx context.Context, ownerID string, limit, offset int) ([]store.File, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, size, chunk_size, created_at, owner_user_id, visibility, status,
		       access_token_hash, access_token_rotated_at, expires_at,
		       password_hash, max_downloads, download_count, share_mode, sha256
		FROM files
		WHERE owner_user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`, ownerID, limit, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.File, error) {
		return scanFileMeta(row)
	})
}
