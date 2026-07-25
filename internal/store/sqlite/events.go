package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

// RecordEvent bumps counters and tracks unique visitors.
func (s *Store) RecordEvent(ctx context.Context, e store.Event) error {
	col := kindColumn(e.Kind)
	if col == "" {
		return fmt.Errorf("unknown event kind %q", e.Kind)
	}
	now := tsArg(time.Now())

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// col comes from the kindColumn whitelist, never from user input.
	if err := affected(tx.ExecContext(ctx, fmt.Sprintf(`
		UPDATE files SET
			%s = %s + 1,
			bytes_served = bytes_served + ?,
			last_access_at = ?
		WHERE id = ?`, col, col), e.Bytes, now, e.FileID)); err != nil {
		return err
	}

	if e.VisitorHash != "" {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO file_visitors (file_id, visitor_hash, first_seen) VALUES (?, ?, ?)
			 ON CONFLICT DO NOTHING`, e.FileID, e.VisitorHash, now)
		if err != nil {
			return mapErr(err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n > 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE files SET unique_visitors = unique_visitors + 1 WHERE id = ?`, e.FileID); err != nil {
				return mapErr(err)
			}
		}
	}

	return tx.Commit()
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

	err = s.db.QueryRowContext(ctx, `
		SELECT views, downloads, ranges, bytes_served, unique_visitors, last_access_at
		FROM files WHERE id = ?`, id).
		Scan(&out.Views, &out.Downloads, &out.Ranges,
			&out.BytesServed, &out.UniqueVisitors, intoTSPtr(&out.LastAccessAt))
	if err != nil {
		return store.FileInspect{}, mapErr(err)
	}
	return out, nil
}
