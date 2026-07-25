package pgx

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mewisme/discloud-go/internal/store"
)

func (s *Store) CreateAPIToken(ctx context.Context, t store.APIToken) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO api_tokens (id, user_id, name, token_hash, scopes, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		t.ID, t.UserID, t.Name, t.TokenHash, t.Scopes, t.ExpiresAt, t.CreatedAt)
	return err
}

// GetAPITokenByHash returns an active (non-revoked) token by hash.
// Caller checks expiry against now.
func (s *Store) GetAPITokenByHash(ctx context.Context, tokenHash string) (store.APIToken, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, token_hash, scopes, expires_at, revoked_at, last_used_at, created_at
		FROM api_tokens WHERE token_hash = $1`, tokenHash)
	t, err := scanAPIToken(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.APIToken{}, store.ErrNotFound
	}
	return t, err
}

// ListAPITokensByUser returns non-revoked tokens for a user, newest first.
func (s *Store) ListAPITokensByUser(ctx context.Context, userID string) ([]store.APIToken, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, token_hash, scopes, expires_at, revoked_at, last_used_at, created_at
		FROM api_tokens
		WHERE user_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.APIToken, error) {
		return scanAPIToken(row)
	})
}

func (s *Store) RevokeAPIToken(ctx context.Context, id, userID string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE api_tokens SET revoked_at = $3
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, id, userID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// RevokeAPITokensByUser revokes every active token for a user (e.g. password change).
func (s *Store) RevokeAPITokensByUser(ctx context.Context, userID string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE api_tokens SET revoked_at = $2
		WHERE user_id = $1 AND revoked_at IS NULL`, userID, now)
	return err
}

func (s *Store) TouchAPITokenLastUsed(ctx context.Context, id string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE api_tokens SET last_used_at = $2 WHERE id = $1`, id, now)
	return err
}

// CountAPITokensByUser counts non-revoked tokens.
func (s *Store) CountAPITokensByUser(ctx context.Context, userID string) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM api_tokens
		WHERE user_id = $1 AND revoked_at IS NULL`, userID).Scan(&n)
	return n, err
}

func scanAPIToken(row scannable) (store.APIToken, error) {
	var t store.APIToken
	var userID string
	err := row.Scan(&t.ID, &userID, &t.Name, &t.TokenHash, &t.Scopes,
		&t.ExpiresAt, &t.RevokedAt, &t.LastUsedAt, &t.CreatedAt)
	if err != nil {
		return store.APIToken{}, err
	}
	t.ID = store.UuidHex(t.ID)
	t.UserID = store.UuidHex(userID)
	if t.Scopes == nil {
		t.Scopes = []string{}
	}
	return t, nil
}
