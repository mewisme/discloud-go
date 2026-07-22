ALTER TABLE users
  ADD COLUMN password_changed_at timestamptz NOT NULL DEFAULT now();
UPDATE users SET password_changed_at = updated_at;

ALTER TABLE sessions
  ADD COLUMN user_agent text NOT NULL DEFAULT '',
  ADD COLUMN ip text NOT NULL DEFAULT '',
  ADD COLUMN last_seen_at timestamptz NOT NULL DEFAULT now();
UPDATE sessions SET last_seen_at = created_at;
