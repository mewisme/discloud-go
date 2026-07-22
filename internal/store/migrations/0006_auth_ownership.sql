-- Users, sessions, and file ownership / visibility / retention.

CREATE TABLE users (
    id            text PRIMARY KEY,
    email         text NOT NULL,
    password_hash text NOT NULL,
    role          text NOT NULL CHECK (role IN ('admin', 'user')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_idx ON users (email);

CREATE TABLE sessions (
    id         text PRIMARY KEY,
    user_id    text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX sessions_token_hash_idx ON sessions (token_hash);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

ALTER TABLE files
    ADD COLUMN owner_user_id text REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN visibility text NOT NULL DEFAULT 'public',
    ADD COLUMN access_token_hash text,
    ADD COLUMN access_token_rotated_at timestamptz,
    ADD COLUMN expires_at timestamptz;

ALTER TABLE files
    ADD CONSTRAINT files_visibility_check CHECK (visibility IN ('public', 'private'));

-- Backfill: anonymous public files expire 7 days after creation.
UPDATE files
SET owner_user_id = NULL,
    visibility = 'public',
    expires_at = created_at + interval '7 days'
WHERE expires_at IS NULL;

ALTER TABLE files
    ALTER COLUMN expires_at SET NOT NULL;

CREATE INDEX files_owner_created_idx ON files (owner_user_id, created_at DESC);
CREATE INDEX files_expires_at_idx ON files (expires_at);
