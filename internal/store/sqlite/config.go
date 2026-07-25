package sqlite

import (
	"context"

	"github.com/mewisme/discloud-go/internal/store"
)

// ListConfig is Postgres-only.
func (s *Store) ListConfig(context.Context) ([]store.ConfigEntry, error) {
	return nil, store.ErrNotSupported
}

// GetConfig is Postgres-only.
func (s *Store) GetConfig(context.Context, string) (store.ConfigEntry, error) {
	return store.ConfigEntry{}, store.ErrNotSupported
}

// SetConfig is Postgres-only.
func (s *Store) SetConfig(context.Context, string, string) error {
	return store.ErrNotSupported
}

// DeleteConfig is Postgres-only.
func (s *Store) DeleteConfig(context.Context, string) error {
	return store.ErrNotSupported
}

// SetConfigBatch is Postgres-only.
func (s *Store) SetConfigBatch(context.Context, map[string]string) error {
	return store.ErrNotSupported
}
