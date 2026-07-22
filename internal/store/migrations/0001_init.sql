-- Final schema (Postgres 18+). Entity IDs are uuid with DEFAULT uuidv7().

CREATE TABLE users (
    id                   uuid PRIMARY KEY DEFAULT uuidv7(),
    username             text NOT NULL,
    password_hash        text NOT NULL,
    role                 text NOT NULL CHECK (role IN ('admin', 'user')),
    default_visibility   text NOT NULL DEFAULT 'public'
        CHECK (default_visibility IN ('public', 'private')),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    password_changed_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_username_idx ON users (username);

CREATE TABLE sessions (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash   text NOT NULL,
    expires_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    user_agent   text NOT NULL DEFAULT '',
    ip           text NOT NULL DEFAULT '',
    last_seen_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX sessions_token_hash_idx ON sessions (token_hash);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

CREATE TABLE files (
    id                       uuid PRIMARY KEY DEFAULT uuidv7(),
    name                     text NOT NULL,
    size                     bigint NOT NULL,
    chunk_size               integer NOT NULL,
    created_at               timestamptz NOT NULL DEFAULT now(),
    views                    bigint NOT NULL DEFAULT 0,
    downloads                bigint NOT NULL DEFAULT 0,
    ranges                   bigint NOT NULL DEFAULT 0,
    bytes_served             bigint NOT NULL DEFAULT 0,
    unique_visitors          bigint NOT NULL DEFAULT 0,
    last_access_at           timestamptz,
    owner_user_id            uuid REFERENCES users (id) ON DELETE SET NULL,
    visibility               text NOT NULL DEFAULT 'public'
        CHECK (visibility IN ('public', 'private')),
    access_token_hash        text,
    access_token_rotated_at  timestamptz,
    expires_at               timestamptz NOT NULL
);

CREATE INDEX files_created_at_idx ON files (created_at DESC);
CREATE INDEX files_owner_created_idx ON files (owner_user_id, created_at DESC);
CREATE INDEX files_expires_at_idx ON files (expires_at);

CREATE TABLE bots (
    id         smallint PRIMARY KEY,
    discord_id text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE chunks (
    file_id    uuid NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    idx        integer NOT NULL,
    message_id text NOT NULL,
    bot_id     smallint REFERENCES bots (id),
    PRIMARY KEY (file_id, idx)
);

CREATE TABLE chunk_store (
    hash       text PRIMARY KEY,
    message_id text NOT NULL,
    size       bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    bot_id     smallint REFERENCES bots (id)
);

CREATE TABLE file_events (
    id           bigserial PRIMARY KEY,
    file_id      uuid NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    kind         text NOT NULL,
    bytes        bigint NOT NULL DEFAULT 0,
    visitor_hash text,
    referrer     text,
    country      text,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX file_events_file_created_idx ON file_events (file_id, created_at DESC);
CREATE INDEX file_events_file_day_idx ON file_events (file_id, created_at);

CREATE TABLE file_visitors (
    file_id      uuid NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    visitor_hash text NOT NULL,
    first_seen   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (file_id, visitor_hash)
);
