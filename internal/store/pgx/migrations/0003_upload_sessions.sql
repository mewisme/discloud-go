-- Upload sessions (resumable) + rename files.status duplicate → reused.

UPDATE files SET status = 'reused' WHERE status = 'duplicate';

ALTER TABLE files DROP CONSTRAINT IF EXISTS files_status_check;
ALTER TABLE files ADD CONSTRAINT files_status_check
    CHECK (status IN ('ready', 'reused'));

CREATE TABLE upload_sessions (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    owner_user_id      uuid REFERENCES users (id) ON DELETE SET NULL,
    resume_token_hash  text NOT NULL,
    file_name          text NOT NULL,
    file_size          bigint NOT NULL CHECK (file_size > 0),
    chunk_size         integer NOT NULL,
    chunk_count        integer NOT NULL CHECK (chunk_count > 0),
    status             text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'uploading', 'completing', 'completed', 'cancelled', 'expired')),
    file_id            uuid REFERENCES files (id) ON DELETE SET NULL,
    client_fingerprint text NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    expires_at         timestamptz NOT NULL
);

CREATE INDEX upload_sessions_expires_idx ON upload_sessions (expires_at)
    WHERE status IN ('pending', 'uploading');
CREATE INDEX upload_sessions_owner_idx ON upload_sessions (owner_user_id, created_at DESC);

CREATE TABLE upload_session_parts (
    upload_id  uuid NOT NULL REFERENCES upload_sessions (id) ON DELETE CASCADE,
    idx        integer NOT NULL,
    chunk_hash text,
    PRIMARY KEY (upload_id, idx)
);
