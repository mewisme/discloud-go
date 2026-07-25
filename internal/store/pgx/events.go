package pgx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mewisme/discloud-go/internal/store"
)

// RecordEvent bumps counters and tracks unique visitors.
func (s *Store) RecordEvent(ctx context.Context, e store.Event) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	col := kindColumn(e.Kind)
	if col == "" {
		return fmt.Errorf("unknown event kind %q", e.Kind)
	}

	tag, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE files SET
			%s = %s + 1,
			bytes_served = bytes_served + $2,
			last_access_at = now()
		WHERE id = $1`, col, col), e.FileID, e.Bytes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}

	if e.VisitorHash != "" {
		ct, err := tx.Exec(ctx, `
			INSERT INTO file_visitors (file_id, visitor_hash) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, e.FileID, e.VisitorHash)
		if err != nil {
			return err
		}
		if ct.RowsAffected() > 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE files SET unique_visitors = unique_visitors + 1 WHERE id = $1`, e.FileID); err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}

func kindColumn(kind string) string {
	switch kind {
	case store.EventView:
		return "views"
	case store.EventDownload:
		return "downloads"
	case store.EventRange:
		return "ranges"
	default:
		return ""
	}
}

// GetFileInspect loads file meta and access counters.
func (s *Store) GetFileInspect(ctx context.Context, id string) (store.FileInspect, error) {
	f, err := s.GetFile(ctx, id)
	if err != nil {
		return store.FileInspect{}, err
	}

	var out store.FileInspect
	out.File = f
	out.ChunkCount = len(f.Parts)

	var lastAccess *time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT views, downloads, ranges, bytes_served, unique_visitors, last_access_at
		FROM files WHERE id = $1`, id).
		Scan(&out.Views, &out.Downloads, &out.Ranges,
			&out.BytesServed, &out.UniqueVisitors, &lastAccess)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.FileInspect{}, store.ErrNotFound
	}
	if err != nil {
		return store.FileInspect{}, err
	}
	out.LastAccessAt = lastAccess
	return out, nil
}
