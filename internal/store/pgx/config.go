package pgx

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mewisme/discloud-go/internal/store"
)

// ListConfig returns all runtime config rows.
func (s *Store) ListConfig(ctx context.Context) ([]store.ConfigEntry, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, value, updated_at FROM config ORDER BY key`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.ConfigEntry, error) {
		var e store.ConfigEntry
		err := row.Scan(&e.Key, &e.Value, &e.UpdatedAt)
		return e, err
	})
}

// GetConfig returns one config row.
func (s *Store) GetConfig(ctx context.Context, key string) (store.ConfigEntry, error) {
	var e store.ConfigEntry
	err := s.pool.QueryRow(ctx,
		`SELECT key, value, updated_at FROM config WHERE key = $1`, key,
	).Scan(&e.Key, &e.Value, &e.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ConfigEntry{}, store.ErrNotFound
	}
	return e, err
}

// SetConfig upserts a config key.
func (s *Store) SetConfig(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO config (key, value, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		key, value)
	return err
}

// DeleteConfig removes a config key (env default applies again).
func (s *Store) DeleteConfig(ctx context.Context, key string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM config WHERE key = $1`, key)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// SetConfigBatch upserts multiple keys in one transaction.
func (s *Store) SetConfigBatch(ctx context.Context, entries map[string]string) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	now := time.Now().UTC()
	for k, v := range entries {
		if _, err := tx.Exec(ctx, `
			INSERT INTO config (key, value, updated_at) VALUES ($1, $2, $3)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`,
			k, v, now); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
