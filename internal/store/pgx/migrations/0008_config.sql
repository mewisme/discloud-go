-- Runtime public settings (Docker/Postgres). Empty table → use env defaults.
CREATE TABLE config (
    key        text PRIMARY KEY,
    value      text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
