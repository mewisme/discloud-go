-- Drop inspect counter (inspect page no longer records itself).
ALTER TABLE files DROP COLUMN IF EXISTS inspects;
DELETE FROM file_events WHERE kind = 'inspect';
