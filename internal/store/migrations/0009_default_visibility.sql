ALTER TABLE users
  ADD COLUMN default_visibility text NOT NULL DEFAULT 'public',
  ADD CONSTRAINT users_default_visibility_check
    CHECK (default_visibility IN ('public', 'private'));
