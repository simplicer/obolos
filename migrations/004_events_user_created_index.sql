-- 004_events_user_created_index.sql
-- Composite index for the hot-path rate-limit count and the usage/charges
-- queries, which all filter by user_id and created_at together.
CREATE INDEX IF NOT EXISTS idx_events_user_created
    ON metering_events (user_id, created_at);
