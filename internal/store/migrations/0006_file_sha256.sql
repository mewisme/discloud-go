-- Whole-file integrity digest (discloud-sha256-v1). Nullable for legacy rows.

ALTER TABLE files ADD COLUMN sha256 text;
