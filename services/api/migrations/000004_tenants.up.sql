-- 000004_tenants.up.sql
-- Tenant boundaries, school configuration, and server-authoritative memberships.

CREATE TYPE tenant_status AS ENUM ('PENDING_SETUP', 'ACTIVE', 'SUSPENDED', 'ARCHIVED');
CREATE TYPE membership_status AS ENUM ('ACTIVE', 'SUSPENDED');

CREATE TABLE tenants (
    id UUID PRIMARY KEY,
    slug CITEXT NOT NULL UNIQUE,
    name VARCHAR(160) NOT NULL,
    status tenant_status NOT NULL DEFAULT 'PENDING_SETUP',
    locale VARCHAR(20) NOT NULL DEFAULT 'en-NG',
    timezone VARCHAR(80) NOT NULL DEFAULT 'Africa/Lagos',
    branding JSONB NOT NULL DEFAULT '{}'::jsonb,
    feature_flags JSONB NOT NULL DEFAULT '{}'::jsonb,
    education_stages TEXT[] NOT NULL DEFAULT ARRAY['EARLY_YEARS', 'PRIMARY', 'JUNIOR_SECONDARY', 'SENIOR_SECONDARY']::TEXT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT tenants_slug_format CHECK (slug::TEXT ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    CONSTRAINT tenants_name_nonempty CHECK (length(trim(name)) BETWEEN 1 AND 160),
    CONSTRAINT tenants_locale_nonempty CHECK (length(trim(locale)) BETWEEN 2 AND 20),
    CONSTRAINT tenants_timezone_nonempty CHECK (length(trim(timezone)) BETWEEN 1 AND 80)
);

CREATE TABLE memberships (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(40) NOT NULL CHECK (role IN (
        'SCHOOL_OWNER', 'SCHOOL_ADMIN', 'PRINCIPAL', 'TEACHER',
        'STUDENT', 'PARENT', 'BURSAR'
    )),
    status membership_status NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, user_id, role),
    UNIQUE (tenant_id, id)
);

CREATE INDEX idx_memberships_user_active ON memberships(user_id, tenant_id) WHERE status = 'ACTIVE';

ALTER TABLE invitations
    ADD CONSTRAINT invitations_tenant_fk
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE audit_events
    ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL;

CREATE INDEX idx_audit_events_tenant_created ON audit_events(tenant_id, created_at);

INSERT INTO schema_migrations (version) VALUES ('000004_tenants')
ON CONFLICT (version) DO NOTHING;
