package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Upload session lifecycle statuses (not files.status).
const (
	UploadPending    = "pending"
	UploadUploading  = "uploading"
	UploadCompleting = "completing"
	UploadCompleted  = "completed"
	UploadCancelled  = "cancelled"
	UploadExpired    = "expired"
)

// ErrPartConflict is returned when a session part is re-registered with a different hash.
var ErrPartConflict = errors.New("upload part hash conflict")

// ErrUploadNotActive is returned when mutating a non-open upload session.
var ErrUploadNotActive = errors.New("upload session not active")

// UploadSession is a resumable upload ledger (bytes live in chunk_store).
type UploadSession struct {
	ID                string
	OwnerUserID       *string
	ResumeTokenHash   string
	FileName          string
	FileSize          int64
	ChunkSize         int
	ChunkCount        int
	Status            string
	FileID            *string
	ClientFingerprint string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ExpiresAt         time.Time
	Parts             map[int]string // idx → chunk hash (registered only)
}

// CreateUploadSession inserts a new pending upload session.
func (s *Store) CreateUploadSession(ctx context.Context, u UploadSession) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO upload_sessions (
			id, owner_user_id, resume_token_hash, file_name, file_size, chunk_size, chunk_count,
			status, client_fingerprint, created_at, updated_at, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		u.ID, u.OwnerUserID, u.ResumeTokenHash, u.FileName, u.FileSize, u.ChunkSize, u.ChunkCount,
		u.Status, u.ClientFingerprint, u.CreatedAt, u.UpdatedAt, u.ExpiresAt)
	return err
}

// GetUploadSession loads a session and its registered parts.
func (s *Store) GetUploadSession(ctx context.Context, id string) (UploadSession, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, owner_user_id, resume_token_hash, file_name, file_size, chunk_size, chunk_count,
		       status, file_id, client_fingerprint, created_at, updated_at, expires_at
		FROM upload_sessions WHERE id = $1`, id)
	u, err := scanUploadSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return UploadSession{}, ErrNotFound
	}
	if err != nil {
		return UploadSession{}, err
	}
	parts, err := s.listUploadParts(ctx, u.ID)
	if err != nil {
		return UploadSession{}, err
	}
	u.Parts = parts
	return u, nil
}

func scanUploadSession(row scannable) (UploadSession, error) {
	var u UploadSession
	var owner *string
	var fileID *string
	err := row.Scan(
		&u.ID, &owner, &u.ResumeTokenHash, &u.FileName, &u.FileSize, &u.ChunkSize, &u.ChunkCount,
		&u.Status, &fileID, &u.ClientFingerprint, &u.CreatedAt, &u.UpdatedAt, &u.ExpiresAt,
	)
	if err != nil {
		return UploadSession{}, err
	}
	u.ID = uuidHex(u.ID)
	if owner != nil {
		h := uuidHex(*owner)
		u.OwnerUserID = &h
	}
	if fileID != nil {
		h := uuidHex(*fileID)
		u.FileID = &h
	}
	return u, nil
}

func (s *Store) listUploadParts(ctx context.Context, uploadID string) (map[int]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT idx, chunk_hash FROM upload_session_parts WHERE upload_id = $1 AND chunk_hash IS NOT NULL`,
		uploadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var idx int
		var hash string
		if err := rows.Scan(&idx, &hash); err != nil {
			return nil, err
		}
		out[idx] = hash
	}
	return out, rows.Err()
}

// CountOpenUploadSessionsByOwner counts pending+uploading sessions for a user.
func (s *Store) CountOpenUploadSessionsByOwner(ctx context.Context, ownerUserID string) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM upload_sessions
		WHERE owner_user_id = $1::uuid AND status IN ('pending', 'uploading')`,
		ownerUserID).Scan(&n)
	return n, err
}

// CountOpenUploadSessionsAnon counts open anonymous sessions whose fingerprint starts with prefix.
func (s *Store) CountOpenUploadSessionsAnon(ctx context.Context, fingerprintPrefix string) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM upload_sessions
		WHERE owner_user_id IS NULL
		  AND status IN ('pending', 'uploading')
		  AND client_fingerprint LIKE $1`,
		fingerprintPrefix+"%").Scan(&n)
	return n, err
}

// RegisterUploadPart sets chunk hash for an index. Idempotent for same hash; conflict otherwise.
func (s *Store) RegisterUploadPart(ctx context.Context, uploadID string, idx int, hash string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var status string
	var chunkCount int
	err = tx.QueryRow(ctx, `
		SELECT status, chunk_count FROM upload_sessions WHERE id = $1 FOR UPDATE`, uploadID).
		Scan(&status, &chunkCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != UploadPending && status != UploadUploading {
		return ErrUploadNotActive
	}
	if idx < 0 || idx >= chunkCount {
		return ErrNotFound
	}

	var existing *string
	err = tx.QueryRow(ctx,
		`SELECT chunk_hash FROM upload_session_parts WHERE upload_id = $1 AND idx = $2`,
		uploadID, idx).Scan(&existing)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx,
			`INSERT INTO upload_session_parts (upload_id, idx, chunk_hash) VALUES ($1,$2,$3)`,
			uploadID, idx, hash)
		if err != nil {
			return err
		}
	} else if existing != nil && *existing != "" {
		if *existing != hash {
			return ErrPartConflict
		}
	} else {
		_, err = tx.Exec(ctx,
			`UPDATE upload_session_parts SET chunk_hash = $3 WHERE upload_id = $1 AND idx = $2`,
			uploadID, idx, hash)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE upload_sessions SET status = $2, updated_at = $3 WHERE id = $1`,
		uploadID, UploadUploading, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// BeginUploadComplete transitions pending/uploading → completing.
// If already completed with file_id, returns the session (idempotent complete).
func (s *Store) BeginUploadComplete(ctx context.Context, uploadID string, now time.Time) (UploadSession, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return UploadSession{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `
		SELECT id, owner_user_id, resume_token_hash, file_name, file_size, chunk_size, chunk_count,
		       status, file_id, client_fingerprint, created_at, updated_at, expires_at
		FROM upload_sessions WHERE id = $1 FOR UPDATE`, uploadID)
	u, err := scanUploadSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return UploadSession{}, ErrNotFound
	}
	if err != nil {
		return UploadSession{}, err
	}
	if u.Status == UploadCompleted && u.FileID != nil {
		if err := tx.Commit(ctx); err != nil {
			return UploadSession{}, err
		}
		parts, err := s.listUploadParts(ctx, u.ID)
		if err != nil {
			return UploadSession{}, err
		}
		u.Parts = parts
		return u, nil
	}
	if u.Status != UploadPending && u.Status != UploadUploading {
		return UploadSession{}, ErrUploadNotActive
	}
	_, err = tx.Exec(ctx, `
		UPDATE upload_sessions SET status = $2, updated_at = $3 WHERE id = $1`,
		uploadID, UploadCompleting, now)
	if err != nil {
		return UploadSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UploadSession{}, err
	}
	parts, err := s.listUploadParts(ctx, u.ID)
	if err != nil {
		return UploadSession{}, err
	}
	u.Parts = parts
	u.Status = UploadCompleting
	u.UpdatedAt = now
	return u, nil
}

// FinishUploadComplete marks session completed and links file_id.
func (s *Store) FinishUploadComplete(ctx context.Context, uploadID, fileID string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE upload_sessions
		SET status = $2, file_id = $3::uuid, updated_at = $4
		WHERE id = $1 AND status = $5`,
		uploadID, UploadCompleted, fileID, now, UploadCompleting)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AbortUploadComplete rolls completing back to uploading after a failed assemble.
func (s *Store) AbortUploadComplete(ctx context.Context, uploadID string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE upload_sessions SET status = $2, updated_at = $3
		WHERE id = $1 AND status = $4`,
		uploadID, UploadUploading, now, UploadCompleting)
	return err
}

// CancelUploadSession marks pending/uploading as cancelled.
func (s *Store) CancelUploadSession(ctx context.Context, uploadID string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE upload_sessions SET status = $2, updated_at = $3
		WHERE id = $1 AND status IN ('pending', 'uploading')`,
		uploadID, UploadCancelled, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var status string
		err := s.pool.QueryRow(ctx, `SELECT status FROM upload_sessions WHERE id = $1`, uploadID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return ErrUploadNotActive
	}
	return nil
}

// ExpireUploadSessions marks expired open sessions. Returns rows affected.
func (s *Store) ExpireUploadSessions(ctx context.Context, now time.Time, limit int) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE upload_sessions
		SET status = $1, updated_at = $2
		WHERE id IN (
			SELECT id FROM upload_sessions
			WHERE expires_at <= $2 AND status IN ('pending', 'uploading')
			ORDER BY expires_at
			LIMIT $3
		)`, UploadExpired, now, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
