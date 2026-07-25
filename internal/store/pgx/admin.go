package pgx

import (
	"context"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

// AdminOverview returns storage/user/upload/traffic aggregates in one round-trip.
// since bounds the upload session 24h counters (updated_at >= since).
func (s *Store) AdminOverview(ctx context.Context, since time.Time) (store.AdminOverview, error) {
	var o store.AdminOverview
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)::bigint FROM files),
			(SELECT COALESCE(SUM(size), 0)::bigint FROM files),
			(SELECT COUNT(*)::bigint FROM chunk_store),
			(SELECT COUNT(*)::bigint FROM users),
			(SELECT COUNT(*)::bigint FROM users WHERE role = 'admin'),
			(SELECT COUNT(*)::bigint FROM upload_sessions
				WHERE status IN ('pending', 'uploading')),
			(SELECT COUNT(*)::bigint FROM upload_sessions
				WHERE status = 'completed' AND updated_at >= $1),
			(SELECT COUNT(*)::bigint FROM upload_sessions
				WHERE status = 'expired' AND updated_at >= $1),
			(SELECT COUNT(*)::bigint FROM upload_sessions
				WHERE status = 'cancelled' AND updated_at >= $1),
			(SELECT COALESCE(SUM(downloads), 0)::bigint FROM files),
			(SELECT COALESCE(SUM(bytes_served), 0)::bigint FROM files)
	`, since).Scan(
		&o.FileCount, &o.TotalBytes, &o.ChunkStoreCount,
		&o.UserCount, &o.AdminCount,
		&o.OpenSessions, &o.Completed24h, &o.Expired24h, &o.Cancelled24h,
		&o.Downloads, &o.BytesServed,
	)
	return o, err
}
