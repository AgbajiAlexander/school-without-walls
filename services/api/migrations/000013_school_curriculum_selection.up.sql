-- A later selection creates a new revision. Historical consumers can pin the
-- exact selection ID and framework version they originally used.
CREATE TABLE school_curriculum_selections (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    session_id UUID NOT NULL,
    stage VARCHAR(20) NOT NULL CHECK (stage IN ('EARLY_YEARS', 'PRIMARY', 'JUNIOR_SECONDARY', 'SENIOR_SECONDARY')),
    curriculum_role VARCHAR(12) NOT NULL CHECK (curriculum_role IN ('PRIMARY', 'ENRICHMENT')),
    revision INTEGER NOT NULL CHECK (revision > 0),
    framework_version_id UUID NOT NULL REFERENCES curriculum_framework_versions(id) ON DELETE RESTRICT,
    selected_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    selected_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (tenant_id, session_id) REFERENCES academic_sessions(tenant_id, id) ON DELETE RESTRICT,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, session_id, stage, curriculum_role, revision)
);

CREATE FUNCTION guard_school_curriculum_selection() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    session_status VARCHAR(16);
    edition_kind VARCHAR(20);
    edition_status VARCHAR(12);
    previous_revision INTEGER;
    previous_edition UUID;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        RAISE EXCEPTION 'curriculum selection history is append-only' USING ERRCODE = '23514';
    END IF;

    SELECT status INTO session_status FROM academic_sessions
    WHERE tenant_id = NEW.tenant_id AND id = NEW.session_id FOR NO KEY UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'curriculum selection needs a session in the same school' USING ERRCODE = '23503';
    END IF;
    IF session_status = 'ARCHIVED' THEN
        RAISE EXCEPTION 'archived sessions cannot change curriculum selection' USING ERRCODE = '23514';
    END IF;

    SELECT framework_kind, status INTO edition_kind, edition_status FROM curriculum_framework_versions
    WHERE id = NEW.framework_version_id FOR SHARE;
    IF edition_status IS DISTINCT FROM 'PUBLISHED' OR NOT EXISTS (
        SELECT 1 FROM curriculum_objectives
        WHERE framework_version_id = NEW.framework_version_id AND stage = NEW.stage
    ) THEN
        RAISE EXCEPTION 'selection needs a published edition with objectives in the selected stage' USING ERRCODE = '23514';
    END IF;
    IF (NEW.curriculum_role = 'ENRICHMENT' AND edition_kind <> 'ENRICHMENT')
       OR (NEW.curriculum_role = 'PRIMARY' AND edition_kind <> 'NIGERIAN_PRIMARY'
           AND NOT (NEW.stage = 'EARLY_YEARS' AND edition_kind = 'DEVELOPMENTAL')) THEN
        RAISE EXCEPTION 'edition does not match the curriculum role and stage' USING ERRCODE = '23514';
    END IF;

    SELECT revision, framework_version_id INTO previous_revision, previous_edition
    FROM school_curriculum_selections
    WHERE tenant_id = NEW.tenant_id AND session_id = NEW.session_id
      AND stage = NEW.stage AND curriculum_role = NEW.curriculum_role
    ORDER BY revision DESC LIMIT 1;
    IF NEW.revision <> COALESCE(previous_revision, 0) + 1 THEN
        RAISE EXCEPTION 'curriculum selection revision must be sequential' USING ERRCODE = '23514';
    END IF;
    IF previous_edition = NEW.framework_version_id THEN
        RAISE EXCEPTION 'this edition is already selected' USING ERRCODE = '23514';
    END IF;
    NEW.selected_at := clock_timestamp();
    RETURN NEW;
END;
$$;

CREATE TRIGGER school_curriculum_selection_history
BEFORE INSERT OR UPDATE OR DELETE ON school_curriculum_selections
FOR EACH ROW EXECUTE FUNCTION guard_school_curriculum_selection();

-- A withdrawn edition remains the current historical choice until an
-- authorized replacement is selected. Consumers must inspect its status.
CREATE VIEW school_current_curriculum_selections AS
SELECT DISTINCT ON (tenant_id, session_id, stage, curriculum_role)
    id, tenant_id, session_id, stage, curriculum_role, revision, framework_version_id, selected_by, selected_at
FROM school_curriculum_selections
ORDER BY tenant_id, session_id, stage, curriculum_role, revision DESC;
