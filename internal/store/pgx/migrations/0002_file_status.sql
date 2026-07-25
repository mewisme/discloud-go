-- Upload outcome badge on files: ready (new) or duplicate (same-user re-upload).

ALTER TABLE files
    ADD COLUMN status text NOT NULL DEFAULT 'ready'
        CHECK (status IN ('ready', 'duplicate'));
