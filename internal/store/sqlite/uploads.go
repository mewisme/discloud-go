package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

// uploadColumns is the projection scanUploadSession expects.
const uploadColumns = `id, owner_user_id, resume_token_hash, file_name, file_size, chunk_size, chunk_count,
	status, file_id, client_fingerprint, created_at, updated_at, expires_at`

// querier is satisfied by both *sql.DB and *sql.Tx. Queries issued while a
// transaction is open must use the transaction: the pool holds a single
// connection, so touching the DB directly would deadlock.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (s *Store) CreateUploadSession(ctx context.Context, u store.UploadSession) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO upload_sessions (
			id, owner_user_id, resume_token_hash, file_name, file_size, chunk_size, chunk_count,
			status, client_fingerprint, created_at, updated_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.OwnerUserID, u.ResumeTokenHash, u.FileName, u.FileSize, u.ChunkSize, u.ChunkCount,
		u.Status, u.ClientFingerprint, tsArg(u.CreatedAt), tsArg(u.UpdatedAt), tsArg(u.ExpiresAt))
	return mapErr(err)
}

func (s *Store) GetUploadSession(ctx context.Context, id string) (store.UploadSession, error) {
	u, err := scanUploadSession(s.db.QueryRowContext(ctx,
		`SELECT `+uploadColumns+` FROM upload_sessions WHERE id = ?`, id))
	if err != nil {
		return store.UploadSession{}, mapErr(err)
	}
	u.Parts, err = listUploadParts(ctx, s.db, u.ID)
	if err != nil {
		return store.UploadSession{}, err
	}
	return u, nil
}

func scanUploadSession(row scannable) (store.UploadSession, error) {
	var u store.UploadSession
	err := row.Scan(
		&u.ID, &u.OwnerUserID, &u.ResumeTokenHash, &u.FileName, &u.FileSize, &u.ChunkSize,
		&u.ChunkCount, &u.Status, &u.FileID, &u.ClientFingerprint,
		intoTS(&u.CreatedAt), intoTS(&u.UpdatedAt), intoTS(&u.ExpiresAt),
	)
	if err != nil {
		return store.UploadSession{}, err
	}
	return u, nil
}

func listUploadParts(ctx context.Context, q querier, uploadID string) (map[int]string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT idx, chunk_hash FROM upload_session_parts WHERE upload_id = ? AND chunk_hash IS NOT NULL`,
		uploadID)
	if err != nil {
		return nil, mapErr(err)
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
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM upload_sessions
		WHERE owner_user_id = ? AND status IN ('pending', 'uploading')`, ownerUserID).Scan(&n)
	return n, mapErr(err)
}

// CountOpenUploadSessionsAnon counts open anonymous sessions whose fingerprint starts with prefix.
func (s *Store) CountOpenUploadSessionsAnon(ctx context.Context, fingerprintPrefix string) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM upload_sessions
		WHERE owner_user_id IS NULL
		  AND status IN ('pending', 'uploading')
		  AND client_fingerprint LIKE ?`, fingerprintPrefix+"%").Scan(&n)
	return n, mapErr(err)
}

// SumOpenUploadBytesByOwner sums file_size of open sessions for a user.
func (s *Store) SumOpenUploadBytesByOwner(ctx context.Context, ownerUserID string) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(file_size), 0) FROM upload_sessions
		WHERE owner_user_id = ? AND status IN ('pending', 'uploading')`, ownerUserID).Scan(&n)
	return n, mapErr(err)
}

// RegisterUploadPart sets chunk hash for an index. Idempotent for same hash; conflict otherwise.
func (s *Store) RegisterUploadPart(ctx context.Context, uploadID string, idx int, hash string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var status string
	var chunkCount int
	err = tx.QueryRowContext(ctx,
		`SELECT status, chunk_count FROM upload_sessions WHERE id = ?`, uploadID).
		Scan(&status, &chunkCount)
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	if err != nil {
		return mapErr(err)
	}
	if status != store.UploadPending && status != store.UploadUploading {
		return store.ErrUploadNotActive
	}
	if idx < 0 || idx >= chunkCount {
		return store.ErrNotFound
	}

	var existing *string
	err = tx.QueryRowContext(ctx,
		`SELECT chunk_hash FROM upload_session_parts WHERE upload_id = ? AND idx = ?`,
		uploadID, idx).Scan(&existing)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO upload_session_parts (upload_id, idx, chunk_hash) VALUES (?, ?, ?)`,
			uploadID, idx, hash); err != nil {
			return mapErr(err)
		}
	case err != nil:
		return mapErr(err)
	case existing != nil && *existing != "":
		if *existing != hash {
			return store.ErrPartConflict
		}
	default:
		if _, err := tx.ExecContext(ctx,
			`UPDATE upload_session_parts SET chunk_hash = ? WHERE upload_id = ? AND idx = ?`,
			hash, uploadID, idx); err != nil {
			return mapErr(err)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE upload_sessions SET status = ?, updated_at = ? WHERE id = ?`,
		store.UploadUploading, tsArg(now), uploadID); err != nil {
		return mapErr(err)
	}
	return tx.Commit()
}

// BeginUploadComplete transitions pending/uploading → completing.
// If already completed with file_id, returns the session (idempotent complete).
func (s *Store) BeginUploadComplete(ctx context.Context, uploadID string, now time.Time) (store.UploadSession, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.UploadSession{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	u, err := scanUploadSession(tx.QueryRowContext(ctx,
		`SELECT `+uploadColumns+` FROM upload_sessions WHERE id = ?`, uploadID))
	if err != nil {
		return store.UploadSession{}, mapErr(err)
	}
	alreadyDone := u.Status == store.UploadCompleted && u.FileID != nil
	if !alreadyDone {
		if u.Status != store.UploadPending && u.Status != store.UploadUploading {
			return store.UploadSession{}, store.ErrUploadNotActive
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE upload_sessions SET status = ?, updated_at = ? WHERE id = ?`,
			store.UploadCompleting, tsArg(now), uploadID); err != nil {
			return store.UploadSession{}, mapErr(err)
		}
		u.Status = store.UploadCompleting
		u.UpdatedAt = now
	}
	u.Parts, err = listUploadParts(ctx, tx, u.ID)
	if err != nil {
		return store.UploadSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return store.UploadSession{}, mapErr(err)
	}
	return u, nil
}

// FinishUploadComplete marks session completed and links file_id.
func (s *Store) FinishUploadComplete(ctx context.Context, uploadID, fileID string, now time.Time) error {
	return affected(s.db.ExecContext(ctx, `
		UPDATE upload_sessions
		SET status = ?, file_id = ?, updated_at = ?
		WHERE id = ? AND status = ?`,
		store.UploadCompleted, fileID, tsArg(now), uploadID, store.UploadCompleting))
}

// AbortUploadComplete rolls completing back to uploading after a failed assemble.
func (s *Store) AbortUploadComplete(ctx context.Context, uploadID string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE upload_sessions SET status = ?, updated_at = ?
		WHERE id = ? AND status = ?`,
		store.UploadUploading, tsArg(now), uploadID, store.UploadCompleting)
	return mapErr(err)
}

// CancelUploadSession marks pending/uploading as cancelled.
func (s *Store) CancelUploadSession(ctx context.Context, uploadID string, now time.Time) error {
	err := affected(s.db.ExecContext(ctx, `
		UPDATE upload_sessions SET status = ?, updated_at = ?
		WHERE id = ? AND status IN ('pending', 'uploading')`,
		store.UploadCancelled, tsArg(now), uploadID))
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	// Nothing updated: either the session is gone or it is no longer active.
	var status string
	if err := s.db.QueryRowContext(ctx,
		`SELECT status FROM upload_sessions WHERE id = ?`, uploadID).Scan(&status); err != nil {
		return mapErr(err)
	}
	return store.ErrUploadNotActive
}

// ExpireUploadSessions marks expired open sessions. Returns rows affected.
func (s *Store) ExpireUploadSessions(ctx context.Context, now time.Time, limit int) (int64, error) {
	ts := tsArg(now)
	res, err := s.db.ExecContext(ctx, `
		UPDATE upload_sessions
		SET status = ?, updated_at = ?
		WHERE id IN (
			SELECT id FROM upload_sessions
			WHERE expires_at <= ? AND status IN ('pending', 'uploading')
			ORDER BY expires_at
			LIMIT ?
		)`, store.UploadExpired, ts, ts, limit)
	if err != nil {
		return 0, mapErr(err)
	}
	return res.RowsAffected()
}
