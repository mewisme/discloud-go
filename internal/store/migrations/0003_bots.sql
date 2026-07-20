-- Bot tokens from DISCORD_BOT_TOKEN (positional id = slot index).
-- Chunks record which bot uploaded so downloads use that token.
CREATE TABLE bots (
    id         smallint PRIMARY KEY, -- 0..n-1 matching token order in env
    discord_id text,                 -- Discord user snowflake (optional)
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE chunks
    ADD COLUMN bot_id smallint REFERENCES bots (id);

ALTER TABLE chunk_store
    ADD COLUMN bot_id smallint REFERENCES bots (id);

-- Lift any messageID@slot locators written before this migration.
INSERT INTO bots (id)
SELECT DISTINCT substring(message_id from '@(\d+)$')::smallint
FROM chunks
WHERE message_id ~ '@\d+$'
ON CONFLICT (id) DO NOTHING;

INSERT INTO bots (id)
SELECT DISTINCT substring(message_id from '@(\d+)$')::smallint
FROM chunk_store
WHERE message_id ~ '@\d+$'
ON CONFLICT (id) DO NOTHING;

UPDATE chunks
SET bot_id = substring(message_id from '@(\d+)$')::smallint,
    message_id = regexp_replace(message_id, '@\d+$', '')
WHERE message_id ~ '@\d+$';

UPDATE chunk_store
SET bot_id = substring(message_id from '@(\d+)$')::smallint,
    message_id = regexp_replace(message_id, '@\d+$', '')
WHERE message_id ~ '@\d+$';
