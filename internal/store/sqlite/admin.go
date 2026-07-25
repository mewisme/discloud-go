package sqlite

import (
	"context"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

// AdminOverview returns storage/user/upload/traffic aggregates in one round-trip.
// since bounds the upload session 24h counters (updated_at >= since).
func (s *Store) AdminOverview(ctx context.Context, since time.Time) (store.AdminOverview, error) {
	ts := tsArg(since)
	var o store.AdminOverview
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM files),
			(SELECT COALESCE(SUM(size), 0) FROM files),
			(SELECT COUNT(*) FROM chunk_store),
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM users WHERE role = 'admin'),
			(SELECT COUNT(*) FROM upload_sessions
				WHERE status IN ('pending', 'uploading')),
			(SELECT COUNT(*) FROM upload_sessions
				WHERE status = 'completed' AND updated_at >= ?),
			(SELECT COUNT(*) FROM upload_sessions
				WHERE status = 'expired' AND updated_at >= ?),
			(SELECT COUNT(*) FROM upload_sessions
				WHERE status = 'cancelled' AND updated_at >= ?),
			(SELECT COALESCE(SUM(downloads), 0) FROM files),
			(SELECT COALESCE(SUM(bytes_served), 0) FROM files)`, ts, ts, ts).
		Scan(
			&o.FileCount, &o.TotalBytes, &o.ChunkStoreCount,
			&o.UserCount, &o.AdminCount,
			&o.OpenSessions, &o.Completed24h, &o.Expired24h, &o.Cancelled24h,
			&o.Downloads, &o.BytesServed,
		)
	return o, mapErr(err)
}
