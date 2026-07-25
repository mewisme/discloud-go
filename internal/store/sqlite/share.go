package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

func (s *Store) UpdateFileShare(ctx context.Context, id string, p store.FileSharePatch) error {
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
		mode = store.ShareModeDownload
	}
	if p.ShareMode != nil {
		mode = *p.ShareMode
	}

	return affected(s.db.ExecContext(ctx, `
		UPDATE files SET
			password_hash = ?,
			expires_at = ?,
			max_downloads = ?,
			share_mode = ?
		WHERE id = ?`, nullText(pw), tsArg(exp), maxDL, mode, id))
}

// IncrementDownloadCount bumps share download_count if under max_downloads.
// Returns ErrDownloadLimit when the cap is already reached, ErrNotFound if missing.
func (s *Store) IncrementDownloadCount(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var max *int
	var count int
	err = tx.QueryRowContext(ctx,
		`SELECT max_downloads, download_count FROM files WHERE id = ?`, id).Scan(&max, &count)
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	if err != nil {
		return mapErr(err)
	}
	if max != nil && count >= *max {
		return store.ErrDownloadLimit
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE files SET download_count = download_count + 1 WHERE id = ?`, id); err != nil {
		return mapErr(err)
	}
	return tx.Commit()
}

// RevokeFile force-expires the file and clears its password. Does not delete the row.
func (s *Store) RevokeFile(ctx context.Context, id string, now time.Time) error {
	return affected(s.db.ExecContext(ctx,
		`UPDATE files SET expires_at = ?, password_hash = NULL WHERE id = ?`, tsArg(now), id))
}
