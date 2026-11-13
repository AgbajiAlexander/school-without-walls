-- 000003_auth_hardening.up.sql
-- Global platform operator grants and shared authentication rate limits.

CREATE TABLE IF NOT EXISTS platform_admins (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS auth_rate_limits (
    action VARCHAR(40) NOT NULL,
    key_hash VARCHAR(64) NOT NULL,
    window_started_at TIMESTAMPTZ NOT NULL,
    attempt_count INTEGER NOT NULL CHECK (attempt_count > 0),
    blocked_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (action, key_hash)
);

CREATE INDEX idx_auth_rate_limits_updated_at ON auth_rate_limits(updated_at);

INSERT INTO schema_migrations (version) VALUES ('000003_auth_hardening')
ON CONFLICT (version) DO NOTHING;
