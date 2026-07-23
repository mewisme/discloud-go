package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrDownloadLimit is returned when a share download cap is exhausted.
var ErrDownloadLimit = errors.New("download limit reached")

// FileSharePatch is a partial update for share settings. Nil fields are unchanged.
// ClearPassword / ClearMaxDownloads force-clear even when the companion pointer is nil.
type FileSharePatch struct {
	PasswordHash      *string // set hash; use ClearPassword to remove
	ClearPassword     bool
	ExpiresAt         *time.Time // set absolute expiry
	MaxDownloads      *int       // set cap
	ClearMaxDownloads bool       // unlimited
	ShareMode         *string
}

// UpdateFileShare applies a partial share settings update.
func (s *Store) UpdateFileShare(ctx context.Context, id string, p FileSharePatch) error {
	f, err := s.GetFile(ctx, id)
	if err != nil {
		return err
	}

	pw := f.PasswordHash
	switch {
	case p.ClearPassword:
		pw = ""
	case p.PasswordHash != nil:
		pw = *p.PasswordHash
	}

	exp := f.ExpiresAt
	if p.ExpiresAt != nil {
		exp = *p.ExpiresAt
	}

	maxDL := f.MaxDownloads
	switch {
	case p.ClearMaxDownloads:
		maxDL = nil
	case p.MaxDownloads != nil:
		n := *p.MaxDownloads
		maxDL = &n
	}

	mode := f.ShareMode
	if mode == "" {
		mode = ShareModeDownload
	}
	if p.ShareMode != nil {
		mode = *p.ShareMode
	}

	var pwAny any
	if pw != "" {
		pwAny = pw
	}
	var maxAny any
	if maxDL != nil {
		maxAny = *maxDL
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE files SET
			password_hash = $2,
			expires_at = $3,
			max_downloads = $4,
			share_mode = $5
		WHERE id = $1`, id, pwAny, exp, maxAny, mode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IncrementDownloadCount bumps share download_count if under max_downloads.
// Returns ErrDownloadLimit when the cap is already reached, ErrNotFound if missing.
func (s *Store) IncrementDownloadCount(ctx context.Context, id string) error {
	var max *int
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT max_downloads, download_count FROM files WHERE id = $1`, id).Scan(&max, &count)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if max != nil && count >= *max {
		return ErrDownloadLimit
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE files
		SET download_count = download_count + 1
		WHERE id = $1
		  AND (max_downloads IS NULL OR download_count < max_downloads)`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Race: another request hit the cap between SELECT and UPDATE.
		return ErrDownloadLimit
	}
	return nil
}

// RevokeFile force-expires the file and clears its password. Does not delete the row.
func (s *Store) RevokeFile(ctx context.Context, id string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE files SET expires_at = $2, password_hash = NULL
		WHERE id = $1`, id, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
