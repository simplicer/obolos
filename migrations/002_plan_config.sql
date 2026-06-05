-- 002_plan_config.sql
-- Per-user entitlement rules (time windows, grant/expiry, rate limit) stored as
-- a single JSONB document. Empty object means "no entitlement rules".
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS plan_config JSONB NOT NULL DEFAULT '{}'::jsonb;
