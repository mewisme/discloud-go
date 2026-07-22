-- Access counters + event log for file inspect / analytics.
ALTER TABLE files
    ADD COLUMN views bigint NOT NULL DEFAULT 0,
    ADD COLUMN downloads bigint NOT NULL DEFAULT 0,
    ADD COLUMN ranges bigint NOT NULL DEFAULT 0,
    ADD COLUMN bytes_served bigint NOT NULL DEFAULT 0,
    ADD COLUMN unique_visitors bigint NOT NULL DEFAULT 0,
    ADD COLUMN last_access_at timestamptz;

CREATE TABLE file_events (
    id            bigserial PRIMARY KEY,
    file_id       text NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    kind          text NOT NULL, -- view | download | range
    bytes         bigint NOT NULL DEFAULT 0,
    visitor_hash  text,
    referrer      text,
    country       text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX file_events_file_created_idx ON file_events (file_id, created_at DESC);
CREATE INDEX file_events_file_day_idx ON file_events (file_id, created_at);

CREATE TABLE file_visitors (
    file_id      text NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    visitor_hash text NOT NULL,
    first_seen   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (file_id, visitor_hash)
);
