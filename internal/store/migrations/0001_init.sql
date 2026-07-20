CREATE TABLE files (
    id         text PRIMARY KEY,
    name       text NOT NULL,
    size       bigint NOT NULL,
    chunk_size integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE chunks (
    file_id    text NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    idx        integer NOT NULL,
    message_id text NOT NULL,
    PRIMARY KEY (file_id, idx)
);

CREATE INDEX files_created_at_idx ON files (created_at DESC);
