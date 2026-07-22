-- Rename users.email → username (immutable login handle).

ALTER TABLE users RENAME COLUMN email TO username;

DROP INDEX IF EXISTS users_email_idx;

CREATE UNIQUE INDEX users_username_idx ON users (username);
