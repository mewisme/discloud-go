package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

// userColumns is the projection scanUser expects.
const userColumns = `id, username, password_hash, role, default_visibility, created_at, updated_at, password_changed_at`

// createUserMu serializes the count-then-insert that makes the first user an
// admin. SQLite has no advisory locks, and a single process owns the database
// file, so a process-level mutex plus the transaction is the whole story.
var createUserMu sync.Mutex

// CreateUser inserts a user. The first user in the DB becomes admin.
func (s *Store) CreateUser(ctx context.Context, id, username, passwordHash string) (store.User, error) {
	createUserMu.Lock()
	defer createUserMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.User{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return store.User{}, mapErr(err)
	}
	role := store.RoleUser
	if count == 0 {
		role = store.RoleAdmin
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, created_at, updated_at, password_changed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, username, passwordHash, role, tsArg(now), tsArg(now), tsArg(now)); err != nil {
		return store.User{}, mapErr(err)
	}
	if err := tx.Commit(); err != nil {
		return store.User{}, mapErr(err)
	}
	return store.User{
		ID: id, Username: username, PasswordHash: passwordHash, Role: role,
		DefaultVisibility: store.VisibilityPublic,
		CreatedAt:         now, UpdatedAt: now, PasswordChangedAt: now,
	}, nil
}

func scanUser(row scannable) (store.User, error) {
	var u store.User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DefaultVisibility,
		intoTS(&u.CreatedAt), intoTS(&u.UpdatedAt), intoTS(&u.PasswordChangedAt))
	if err != nil {
		return store.User{}, err
	}
	return u, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (store.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE username = ?`, username))
	if err != nil {
		return store.User{}, mapErr(err)
	}
	return u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (store.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = ?`, id))
	if err != nil {
		return store.User{}, mapErr(err)
	}
	return u, nil
}

func (s *Store) CreateSession(ctx context.Context, sess store.Session) error {
	lastSeen := sess.LastSeenAt
	if lastSeen.IsZero() {
		lastSeen = sess.CreatedAt
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at, user_agent, ip, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.TokenHash, tsArg(sess.ExpiresAt), tsArg(sess.CreatedAt),
		sess.UserAgent, sess.IP, tsArg(lastSeen))
	return mapErr(err)
}

// GetUserBySessionHash returns the user for a still-valid session token hash.
func (s *Store) GetUserBySessionHash(ctx context.Context, tokenHash string, now time.Time) (store.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.password_hash, u.role, u.default_visibility,
		       u.created_at, u.updated_at, u.password_changed_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, tokenHash, tsArg(now)))
	if err != nil {
		return store.User{}, mapErr(err)
	}
	return u, nil
}

// GetSessionByTokenHash returns a still-valid session by token hash.
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string, now time.Time) (store.Session, error) {
	var sess store.Session
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, expires_at, created_at, user_agent, ip, last_seen_at
		FROM sessions
		WHERE token_hash = ? AND expires_at > ?`, tokenHash, tsArg(now)).
		Scan(&sess.ID, &sess.UserID, &sess.TokenHash, intoTS(&sess.ExpiresAt),
			intoTS(&sess.CreatedAt), &sess.UserAgent, &sess.IP, intoTS(&sess.LastSeenAt))
	if err != nil {
		return store.Session{}, mapErr(err)
	}
	return sess, nil
}

func (s *Store) TouchSession(ctx context.Context, tokenHash, ip, userAgent string, now time.Time) error {
	ts := tsArg(now)
	return affected(s.db.ExecContext(ctx, `
		UPDATE sessions SET last_seen_at = ?, ip = ?, user_agent = ?
		WHERE token_hash = ? AND expires_at > ?`, ts, ip, userAgent, tokenHash, ts))
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return mapErr(err)
}

// DeleteSessionsByUserID removes every session for a user (e.g. after password change).
func (s *Store) DeleteSessionsByUserID(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return mapErr(err)
}

// UpdatePasswordHash sets a new Argon2id hash and bumps updated_at / password_changed_at.
func (s *Store) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	now := tsArg(time.Now())
	return affected(s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ?, password_changed_at = ? WHERE id = ?`,
		passwordHash, now, now, userID))
}

// UpdateDefaultVisibility sets the user's preferred visibility for new uploads.
func (s *Store) UpdateDefaultVisibility(ctx context.Context, userID, visibility string) error {
	return affected(s.db.ExecContext(ctx,
		`UPDATE users SET default_visibility = ?, updated_at = ? WHERE id = ?`,
		visibility, tsArg(time.Now()), userID))
}

func (s *Store) OwnerFileStats(ctx context.Context, ownerID string, now time.Time, soonWithin time.Duration) (store.OwnerStats, error) {
	var st store.OwnerStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(size), 0),
			COALESCE(SUM(CASE WHEN visibility = 'public' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN visibility = 'private' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN expires_at > ? AND expires_at <= ? THEN 1 ELSE 0 END), 0)
		FROM files
		WHERE owner_user_id = ?`, tsArg(now), tsArg(now.Add(soonWithin)), ownerID).
		Scan(&st.FileCount, &st.TotalBytes, &st.PublicCount, &st.PrivateCount, &st.ExpiringSoonCount)
	return st, mapErr(err)
}

// UpdateVisibility sets public/private and clears or sets the access token hash.
// tokenHash is ignored (and cleared) when visibility is public.
func (s *Store) UpdateVisibility(ctx context.Context, id, visibility string, tokenHash *string, rotatedAt *time.Time) error {
	var hash any
	var rotated any
	if visibility == store.VisibilityPrivate && tokenHash != nil {
		hash = *tokenHash
		rotated = tsPtrArg(rotatedAt)
	}
	return affected(s.db.ExecContext(ctx, `
		UPDATE files SET visibility = ?, access_token_hash = ?, access_token_rotated_at = ?
		WHERE id = ?`, visibility, hash, rotated, id))
}

// RotateAccessToken sets a new token hash for a private file.
func (s *Store) RotateAccessToken(ctx context.Context, id, tokenHash string, rotatedAt time.Time) error {
	return affected(s.db.ExecContext(ctx, `
		UPDATE files SET access_token_hash = ?, access_token_rotated_at = ?
		WHERE id = ? AND visibility = 'private'`, tokenHash, tsArg(rotatedAt), id))
}

// DeleteFile removes a file row (cascades chunks/visitors). Does not touch Discord.
func (s *Store) DeleteFile(ctx context.Context, id string) error {
	return affected(s.db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id))
}

// DeleteExpiredFiles deletes up to limit expired files.
func (s *Store) DeleteExpiredFiles(ctx context.Context, now time.Time, limit int) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM files WHERE id IN (
			SELECT id FROM files WHERE expires_at <= ?
			ORDER BY expires_at
			LIMIT ?
		)`, tsArg(now), limit)
	if err != nil {
		return 0, mapErr(err)
	}
	return res.RowsAffected()
}

// ExtendExpiration bumps expires_at by ext, capped at now+cap.
func (s *Store) ExtendExpiration(ctx context.Context, id string, now time.Time, ext, capDur time.Duration) (time.Time, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	var current time.Time
	err = tx.QueryRowContext(ctx,
		`SELECT expires_at FROM files WHERE id = ? AND expires_at > ?`, id, tsArg(now)).
		Scan(intoTS(&current))
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, store.ErrNotFound
	}
	if err != nil {
		return time.Time{}, mapErr(err)
	}
	// LEAST(expires_at + ext, now + cap) — SQLite has no interval arithmetic
	// on text timestamps, so compute it here.
	next := current.Add(ext)
	if capAt := now.Add(capDur); next.After(capAt) {
		next = capAt
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE files SET expires_at = ? WHERE id = ?`, tsArg(next), id); err != nil {
		return time.Time{}, mapErr(err)
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, mapErr(err)
	}
	return next.UTC(), nil
}
