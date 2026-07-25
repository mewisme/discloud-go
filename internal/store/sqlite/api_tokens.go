package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

// apiTokenColumns is the projection scanAPIToken expects.
const apiTokenColumns = `id, user_id, name, token_hash, scopes, expires_at, revoked_at, last_used_at, created_at`

func (s *Store) CreateAPIToken(ctx context.Context, t store.APIToken) error {
	scopes := t.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	raw, err := json.Marshal(scopes)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO api_tokens (id, user_id, name, token_hash, scopes, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.UserID, t.Name, t.TokenHash, string(raw), tsPtrArg(t.ExpiresAt), tsArg(t.CreatedAt))
	return mapErr(err)
}

// GetAPITokenByHash returns a token by hash (revoked or not).
// Caller checks revocation and expiry.
func (s *Store) GetAPITokenByHash(ctx context.Context, tokenHash string) (store.APIToken, error) {
	t, err := scanAPIToken(s.db.QueryRowContext(ctx,
		`SELECT `+apiTokenColumns+` FROM api_tokens WHERE token_hash = ?`, tokenHash))
	if err != nil {
		return store.APIToken{}, mapErr(err)
	}
	return t, nil
}

// ListAPITokensByUser returns non-revoked tokens for a user, newest first.
func (s *Store) ListAPITokensByUser(ctx context.Context, userID string) ([]store.APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+apiTokenColumns+`
		FROM api_tokens
		WHERE user_id = ? AND revoked_at IS NULL
		ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []store.APIToken
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeAPIToken(ctx context.Context, id, userID string, now time.Time) error {
	return affected(s.db.ExecContext(ctx, `
		UPDATE api_tokens SET revoked_at = ?
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, tsArg(now), id, userID))
}

// RevokeAPITokensByUser revokes every active token for a user (e.g. password change).
func (s *Store) RevokeAPITokensByUser(ctx context.Context, userID string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE api_tokens SET revoked_at = ?
		WHERE user_id = ? AND revoked_at IS NULL`, tsArg(now), userID)
	return mapErr(err)
}

func (s *Store) TouchAPITokenLastUsed(ctx context.Context, id string, now time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, tsArg(now), id)
	return mapErr(err)
}

// CountAPITokensByUser counts non-revoked tokens.
func (s *Store) CountAPITokensByUser(ctx context.Context, userID string) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_tokens WHERE user_id = ? AND revoked_at IS NULL`, userID).Scan(&n)
	return n, mapErr(err)
}

func scanAPIToken(row scannable) (store.APIToken, error) {
	var t store.APIToken
	var scopes string
	err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &scopes,
		intoTSPtr(&t.ExpiresAt), intoTSPtr(&t.RevokedAt), intoTSPtr(&t.LastUsedAt),
		intoTS(&t.CreatedAt))
	if err != nil {
		return store.APIToken{}, err
	}
	if scopes != "" {
		if err := json.Unmarshal([]byte(scopes), &t.Scopes); err != nil {
			return store.APIToken{}, fmt.Errorf("sqlite: decode api_tokens.scopes: %w", err)
		}
	}
	if t.Scopes == nil {
		t.Scopes = []string{}
	}
	return t, nil
}
