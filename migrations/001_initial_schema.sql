-- Metering Service - Initial Schema
-- Version: 001

CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(255) PRIMARY KEY,
    plan_type VARCHAR(50) NOT NULL DEFAULT 'prepaid',
    balance BIGINT NOT NULL DEFAULT 0,
    overage_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS metering_events (
    id UUID PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL REFERENCES users(id),
    api VARCHAR(255) NOT NULL,
    endpoint VARCHAR(255) NOT NULL,
    base_cost BIGINT NOT NULL,
    applied_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    final_cost BIGINT NOT NULL,
    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    plan_type VARCHAR(50) NOT NULL,
    overage BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_events_user_id ON metering_events(user_id);
CREATE INDEX IF NOT EXISTS idx_events_created_at ON metering_events(created_at);
CREATE INDEX IF NOT EXISTS idx_events_api ON metering_events(api);
