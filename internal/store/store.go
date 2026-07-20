// Package store persists file metadata in PostgreSQL.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrNotFound is returned when a file id does not exist.
var ErrNotFound = errors.New("file not found")

// File is a stored file: its metadata plus the ordered Discord message ids
// holding each chunk.
type File struct {
	ID         string    `json:"fileId"`
	Name       string    `json:"fileName"`
	Size       int64     `json:"fileSize"`
	ChunkSize  int64     `json:"chunkSize"`
	CreatedAt  time.Time `json:"createdAt"`
	MessageIDs []string  `json:"-"`
}

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

// CreateFile inserts the file row and its chunk rows in one transaction.
func (s *Store) CreateFile(ctx context.Context, f File) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	if _, err := tx.Exec(ctx,
		`INSERT INTO files (id, name, size, chunk_size, created_at) VALUES ($1, $2, $3, $4, $5)`,
		f.ID, f.Name, f.Size, f.ChunkSize, f.CreatedAt); err != nil {
		return fmt.Errorf("insert file: %w", err)
	}
	rows := make([][]any, len(f.MessageIDs))
	for i, msgID := range f.MessageIDs {
		rows[i] = []any{f.ID, i, msgID}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"chunks"},
		[]string{"file_id", "idx", "message_id"}, pgx.CopyFromRows(rows)); err != nil {
		return fmt.Errorf("insert chunks: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Store) GetFile(ctx context.Context, id string) (File, error) {
	var f File
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, size, chunk_size, created_at FROM files WHERE id = $1`, id).
		Scan(&f.ID, &f.Name, &f.Size, &f.ChunkSize, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT message_id FROM chunks WHERE file_id = $1 ORDER BY idx`, id)
	if err != nil {
		return File{}, err
	}
	f.MessageIDs, err = pgx.CollectRows(rows, pgx.RowTo[string])
	return f, err
}

// Chunk is one entry in the content-addressed chunk store.
type Chunk struct {
	Hash      string
	MessageID string
	Size      int64
}

// HasChunk reports whether a chunk with this hash was already uploaded.
func (s *Store) HasChunk(ctx context.Context, hash string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM chunk_store WHERE hash = $1)`, hash).Scan(&exists)
	return exists, err
}

// PutChunk records an uploaded chunk; concurrent duplicate uploads are benign.
func (s *Store) PutChunk(ctx context.Context, c Chunk) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO chunk_store (hash, message_id, size) VALUES ($1, $2, $3) ON CONFLICT (hash) DO NOTHING`,
		c.Hash, c.MessageID, c.Size)
	return err
}

// GetChunks resolves hashes to stored chunks; missing hashes are absent from
// the returned map.
func (s *Store) GetChunks(ctx context.Context, hashes []string) (map[string]Chunk, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT hash, message_id, size FROM chunk_store WHERE hash = ANY($1)`, hashes)
	if err != nil {
		return nil, err
	}
	chunks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Chunk, error) {
		var c Chunk
		err := row.Scan(&c.Hash, &c.MessageID, &c.Size)
		return c, err
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]Chunk, len(chunks))
	for _, c := range chunks {
		out[c.Hash] = c
	}
	return out, nil
}

func (s *Store) ListFiles(ctx context.Context, limit int) ([]File, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, size, chunk_size, created_at FROM files ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (File, error) {
		var f File
		err := row.Scan(&f.ID, &f.Name, &f.Size, &f.ChunkSize, &f.CreatedAt)
		return f, err
	})
}
