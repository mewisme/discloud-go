-- Share controls on files: password, download cap, share mode.

ALTER TABLE files
  ADD COLUMN password_hash   text,
  ADD COLUMN max_downloads   integer
    CHECK (max_downloads IS NULL OR max_downloads > 0),
  ADD COLUMN download_count  integer NOT NULL DEFAULT 0,
  ADD COLUMN share_mode      text NOT NULL DEFAULT 'download'
    CHECK (share_mode IN ('view', 'download'));
