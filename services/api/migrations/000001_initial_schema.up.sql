-- 000001_initial_schema.up.sql
-- Baseline migration for School Without Walls
-- Enables required PostgreSQL extensions and migration audit history

CREATE EXTENSION IF NOT EXISTS "citext";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Foundational schema migration tracking table for local / CI migration verification
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(64) PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO schema_migrations (version) VALUES ('000001_initial_schema')
ON CONFLICT (version) DO NOTHING;
