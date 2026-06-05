-- 003_event_request_id.sql
-- Idempotency key for charges. A caller may send X-Request-Id; a retry with the
-- same (user_id, request_id) must not charge twice. NULL request_ids are
-- distinct in a unique index, so unkeyed charges are never deduplicated.
ALTER TABLE metering_events
    ADD COLUMN IF NOT EXISTS request_id VARCHAR(255);

CREATE UNIQUE INDEX IF NOT EXISTS idx_events_user_request
    ON metering_events (user_id, request_id);
