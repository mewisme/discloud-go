-- file_events was never written; inspect analytics use files counters + file_visitors.
DROP INDEX IF EXISTS file_events_file_day_idx;
DROP INDEX IF EXISTS file_events_file_created_idx;
DROP TABLE IF EXISTS file_events;
