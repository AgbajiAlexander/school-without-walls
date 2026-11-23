CREATE TABLE school_notices (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    published_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title VARCHAR(160) NOT NULL CHECK (length(btrim(title)) > 0),
    body VARCHAR(4000) NOT NULL CHECK (length(btrim(body)) > 0),
    audience TEXT NOT NULL,
    target_role TEXT,
    target_stage TEXT,
    target_class_id UUID,
    published_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (tenant_id, target_class_id) REFERENCES classes (tenant_id, id) ON DELETE RESTRICT,
    CHECK (CASE audience
        WHEN 'SCHOOL' THEN target_role IS NULL AND target_stage IS NULL AND target_class_id IS NULL
        WHEN 'ROLE' THEN target_role IS NOT NULL AND target_stage IS NULL AND target_class_id IS NULL
        WHEN 'STAGE' THEN target_role IS NULL AND target_stage IS NOT NULL AND target_class_id IS NULL
        WHEN 'CLASS' THEN target_role IS NULL AND target_stage IS NULL AND target_class_id IS NOT NULL
        ELSE FALSE END),
    CHECK (target_role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL','TEACHER','BURSAR','STUDENT','PARENT')),
    CHECK (target_stage IN ('EARLY_YEARS','PRIMARY','JUNIOR_SECONDARY','SENIOR_SECONDARY'))
);
CREATE INDEX school_notices_recent ON school_notices (tenant_id, published_at DESC, id DESC);
