package pgx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mewisme/discloud-go/internal/store"
)

// createUserLock is a fixed advisory lock key for first-user = admin.
const createUserLock int64 = 0x646973636C6F7564 // "discloud" truncated

// CreateUser inserts a user. The first user in the DB becomes admin (advisory lock).
func (s *Store) CreateUser(ctx context.Context, id, username, passwordHash string) (store.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.User{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, createUserLock); err != nil {
		return store.User{}, err
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return store.User{}, err
	}
	role := store.RoleUser
	if count == 0 {
		role = store.RoleAdmin
	}
	now := time.Now().UTC()
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, username, password_hash, role, created_at, updated_at, password_changed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, username, passwordHash, role, now, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return store.User{}, store.ErrConflict
		}
		return store.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.User{}, err
	}
	return store.User{
		ID: id, Username: username, PasswordHash: passwordHash, Role: role,
		DefaultVisibility: store.VisibilityPublic,
		CreatedAt:         now, UpdatedAt: now, PasswordChangedAt: now,
	}, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (store.User, error) {
	var u store.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, role, default_visibility, created_at, updated_at, password_changed_at
		 FROM users WHERE username = $1`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DefaultVisibility,
			&u.CreatedAt, &u.UpdatedAt, &u.PasswordChangedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, store.ErrNotFound
	}
	if err != nil {
		return store.User{}, err
	}
	u.ID = store.UuidHex(u.ID)
	return u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (store.User, error) {
	var u store.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, role, default_visibility, created_at, updated_at, password_changed_at
		 FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DefaultVisibility,
			&u.CreatedAt, &u.UpdatedAt, &u.PasswordChangedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, store.ErrNotFound
	}
	if err != nil {
		return store.User{}, err
	}
	u.ID = store.UuidHex(u.ID)
	return u, nil
}

func (s *Store) CreateSession(ctx context.Context, sess store.Session) error {
	lastSeen := sess.LastSeenAt
	if lastSeen.IsZero() {
		lastSeen = sess.CreatedAt
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at, user_agent, ip, last_seen_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		sess.ID, sess.UserID, sess.TokenHash, sess.ExpiresAt, sess.CreatedAt,
		sess.UserAgent, sess.IP, lastSeen)
	return err
}

// GetUserBySessionHash returns the user for a still-valid session token hash.
func (s *Store) GetUserBySessionHash(ctx context.Context, tokenHash string, now time.Time) (store.User, error) {
	var u store.User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.username, u.password_hash, u.role, u.default_visibility, u.created_at, u.updated_at, u.password_changed_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > $2`, tokenHash, now).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DefaultVisibility,
			&u.CreatedAt, &u.UpdatedAt, &u.PasswordChangedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, store.ErrNotFound
	}
	if err != nil {
		return store.User{}, err
	}
	u.ID = store.UuidHex(u.ID)
	return u, nil
}

// GetSessionByTokenHash returns a still-valid session by token hash.
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string, now time.Time) (store.Session, error) {
	var sess store.Session
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, created_at, user_agent, ip, last_seen_at
		FROM sessions
		WHERE token_hash = $1 AND expires_at > $2`, tokenHash, now).
		Scan(&sess.ID, &sess.UserID, &sess.TokenHash, &sess.ExpiresAt, &sess.CreatedAt,
			&sess.UserAgent, &sess.IP, &sess.LastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Session{}, store.ErrNotFound
	}
	if err != nil {
		return store.Session{}, err
	}
	sess.ID = store.UuidHex(sess.ID)
	sess.UserID = store.UuidHex(sess.UserID)
	return sess, nil
}

func (s *Store) TouchSession(ctx context.Context, tokenHash, ip, userAgent string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE sessions SET last_seen_at = $2, ip = $3, user_agent = $4
		WHERE token_hash = $1 AND expires_at > $2`,
		tokenHash, now, ip, userAgent)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

// DeleteSessionsByUserID removes every session for a user (e.g. after password change).
func (s *Store) DeleteSessionsByUserID(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

// UpdatePasswordHash sets a new Argon2id hash and bumps updated_at / password_changed_at.
func (s *Store) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = $3, password_changed_at = $3 WHERE id = $1`,
		userID, passwordHash, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// UpdateDefaultVisibility sets the user's preferred visibility for new uploads.
func (s *Store) UpdateDefaultVisibility(ctx context.Context, userID, visibility string) error {
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx,
		`UPDATE users SET default_visibility = $2, updated_at = $3 WHERE id = $1`,
		userID, visibility, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) OwnerFileStats(ctx context.Context, ownerID string, now time.Time, soonWithin time.Duration) (store.OwnerStats, error) {
	soon := now.Add(soonWithin)
	var st store.OwnerStats
	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*)::bigint,
			COALESCE(SUM(size), 0)::bigint,
			COUNT(*) FILTER (WHERE visibility = 'public')::bigint,
			COUNT(*) FILTER (WHERE visibility = 'private')::bigint,
			COUNT(*) FILTER (WHERE expires_at > $2 AND expires_at <= $3)::bigint
		FROM files
		WHERE owner_user_id = $1`, ownerID, now, soon).
		Scan(&st.FileCount, &st.TotalBytes, &st.PublicCount, &st.PrivateCount, &st.ExpiringSoonCount)
	return st, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// UpdateVisibility sets public/private and clears or sets the access token hash.
// tokenHash is ignored (and cleared) when visibility is public.
func (s *Store) UpdateVisibility(ctx context.Context, id, visibility string, tokenHash *string, rotatedAt *time.Time) error {
	var hash any
	var rotated any
	if visibility == store.VisibilityPrivate && tokenHash != nil {
		hash = *tokenHash
		if rotatedAt != nil {
			rotated = *rotatedAt
		}
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE files SET visibility = $2, access_token_hash = $3, access_token_rotated_at = $4
		WHERE id = $1`, id, visibility, hash, rotated)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// RotateAccessToken sets a new token hash for a private file.
func (s *Store) RotateAccessToken(ctx context.Context, id, tokenHash string, rotatedAt time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE files SET access_token_hash = $2, access_token_rotated_at = $3
		WHERE id = $1 AND visibility = 'private'`, id, tokenHash, rotatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// DeleteFile removes a file row (cascades chunks/events/visitors). Does not touch Discord.
func (s *Store) DeleteFile(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM files WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

const cleanupLock int64 = 0x646973636C65616E // "disclean" truncated-ish

// DeleteExpiredFiles deletes up to limit expired files under an advisory lock.
func (s *Store) DeleteExpiredFiles(ctx context.Context, now time.Time, limit int) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, cleanupLock); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM files WHERE id IN (
			SELECT id FROM files WHERE expires_at <= $1
			ORDER BY expires_at
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)`, now, limit)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ExtendExpiration bumps expires_at by ext, capped at now+cap.
func (s *Store) ExtendExpiration(ctx context.Context, id string, now time.Time, ext, capDur time.Duration) (time.Time, error) {
	capAt := now.Add(capDur)
	var expires time.Time
	err := s.pool.QueryRow(ctx, `
		UPDATE files
		SET expires_at = LEAST(expires_at + $2::interval, $3::timestamptz)
		WHERE id = $1 AND expires_at > $4
		RETURNING expires_at`,
		id, fmt.Sprintf("%d seconds", int64(ext.Seconds())), capAt, now).Scan(&expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, store.ErrNotFound
	}
	return expires, err
}
