-- Content-addressed chunk dedup: one row per unique chunk ever uploaded.
-- Chunked uploads check here first and skip the Discord upload on a hit.
CREATE TABLE chunk_store (
    hash       text PRIMARY KEY, -- hex sha-256 of the chunk bytes
    message_id text NOT NULL,
    size       bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
