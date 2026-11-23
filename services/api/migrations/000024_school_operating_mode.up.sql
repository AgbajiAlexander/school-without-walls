ALTER TABLE tenants ADD COLUMN operating_mode TEXT NOT NULL DEFAULT 'ON_SITE',
    ADD COLUMN mode_starts_at TIMESTAMPTZ,
    ADD COLUMN mode_ends_at TIMESTAMPTZ,
    ADD COLUMN mode_reason VARCHAR(500),
    ADD COLUMN mode_help VARCHAR(1000),
    ADD COLUMN mode_schedule_url VARCHAR(2048),
    ADD CONSTRAINT tenants_operating_mode_check CHECK (
        (operating_mode = 'ON_SITE' AND mode_starts_at IS NULL AND mode_ends_at IS NULL
            AND mode_reason IS NULL AND mode_help IS NULL AND mode_schedule_url IS NULL)
        OR (operating_mode IN ('HYBRID','REMOTE') AND mode_starts_at IS NOT NULL AND mode_ends_at IS NOT NULL
            AND isfinite(mode_starts_at) AND isfinite(mode_ends_at) AND mode_starts_at < mode_ends_at
            AND mode_reason IS NOT NULL AND length(btrim(mode_reason)) > 0)
    );
