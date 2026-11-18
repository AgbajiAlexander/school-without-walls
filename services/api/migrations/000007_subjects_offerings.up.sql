-- SWW-004C: tenant-specific subjects, term offerings, and teacher assignments.
-- Forward-only migration. UUIDs are created by the Go application.
-- An assignment is not a permission grant: future teacher-scoped reads/writes
-- must recheck active TEACHER membership and active offering/parent records.

CREATE TABLE subjects (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    code VARCHAR(32) NOT NULL,
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    status VARCHAR(16) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, code),
    CONSTRAINT subjects_code_valid CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{0,31}$'),
    CONSTRAINT subjects_name_valid CHECK (length(trim(name)) BETWEEN 1 AND 100),
    CONSTRAINT subjects_description_valid CHECK (description IS NULL OR length(description) <= 500),
    CONSTRAINT subjects_status_valid CHECK (status IN ('ACTIVE', 'ARCHIVED'))
);

CREATE INDEX subjects_tenant_status_name ON subjects(tenant_id, status, name);

CREATE TABLE subject_offerings (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    subject_id UUID NOT NULL,
    term_id UUID NOT NULL,
    class_id UUID NOT NULL,
    class_arm_id UUID,
    status VARCHAR(16) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, id),
    CONSTRAINT subject_offerings_subject_fk FOREIGN KEY (tenant_id, subject_id)
        REFERENCES subjects (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT subject_offerings_term_fk FOREIGN KEY (tenant_id, term_id)
        REFERENCES academic_terms (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT subject_offerings_class_fk FOREIGN KEY (tenant_id, class_id)
        REFERENCES classes (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT subject_offerings_arm_fk FOREIGN KEY (tenant_id, class_arm_id)
        REFERENCES class_arms (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT subject_offerings_status_valid CHECK (status IN ('ACTIVE', 'ARCHIVED'))
);

-- NULLS NOT DISTINCT ensures one class-wide offering, not unlimited duplicates.
-- The CI PostgreSQL version is 16, which supports this syntax.
CREATE UNIQUE INDEX subject_offerings_unique_scope
    ON subject_offerings (tenant_id, term_id, class_id, class_arm_id, subject_id)
    NULLS NOT DISTINCT;
CREATE INDEX subject_offerings_tenant_term_class
    ON subject_offerings (tenant_id, term_id, class_id, status);

CREATE TABLE teacher_assignments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    offering_id UUID NOT NULL,
    teacher_membership_id UUID NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, id),
    CONSTRAINT teacher_assignments_offering_fk FOREIGN KEY (tenant_id, offering_id)
        REFERENCES subject_offerings (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT teacher_assignments_membership_fk FOREIGN KEY (tenant_id, teacher_membership_id)
        REFERENCES memberships (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT teacher_assignments_status_valid CHECK (status IN ('ACTIVE', 'INACTIVE'))
);

CREATE UNIQUE INDEX teacher_assignments_one_active_teacher_offering
    ON teacher_assignments (tenant_id, offering_id, teacher_membership_id)
    WHERE status = 'ACTIVE';
CREATE INDEX teacher_assignments_membership_active
    ON teacher_assignments (tenant_id, teacher_membership_id, offering_id)
    WHERE status = 'ACTIVE';

CREATE FUNCTION academic_guard_subject_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'subjects cannot be deleted'
            USING ERRCODE = '23514', CONSTRAINT = 'subject_delete_forbidden';
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id THEN
            RAISE EXCEPTION 'subject identity cannot change'
                USING ERRCODE = '23514', CONSTRAINT = 'subject_identity_immutable';
        END IF;
        IF OLD.status = 'ARCHIVED' THEN
            RAISE EXCEPTION 'archived subject is immutable'
                USING ERRCODE = '23514', CONSTRAINT = 'subject_archived_immutable';
        END IF;
        IF NEW.status = 'ARCHIVED' AND
           (NEW.code IS DISTINCT FROM OLD.code OR NEW.name IS DISTINCT FROM OLD.name OR
            NEW.description IS DISTINCT FROM OLD.description) THEN
            RAISE EXCEPTION 'archive operation may only change status'
                USING ERRCODE = '23514', CONSTRAINT = 'subject_archive_only';
        END IF;
        NEW.updated_at := CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER subjects_guard_mutation
    BEFORE INSERT OR UPDATE OR DELETE ON subjects
    FOR EACH ROW EXECUTE FUNCTION academic_guard_subject_mutation();

CREATE FUNCTION academic_guard_offering_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    subject_status VARCHAR(16);
    class_status VARCHAR(16);
    class_session UUID;
    term_session UUID;
    session_status VARCHAR(16);
    arm_status VARCHAR(16);
    arm_class UUID;
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'subject offerings cannot be deleted'
            USING ERRCODE = '23514', CONSTRAINT = 'offering_delete_forbidden';
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id OR
           NEW.subject_id <> OLD.subject_id OR NEW.term_id <> OLD.term_id OR
           NEW.class_id <> OLD.class_id OR
           NEW.class_arm_id IS DISTINCT FROM OLD.class_arm_id THEN
            RAISE EXCEPTION 'offering identity and placement cannot change'
                USING ERRCODE = '23514', CONSTRAINT = 'offering_identity_immutable';
        END IF;
        IF OLD.status = 'ARCHIVED' THEN
            RAISE EXCEPTION 'archived offering is immutable'
                USING ERRCODE = '23514', CONSTRAINT = 'offering_archived_immutable';
        END IF;
        NEW.updated_at := CURRENT_TIMESTAMP;
    END IF;

    -- Archiving remains possible when a previously linked parent was archived.
    IF NEW.status = 'ARCHIVED' THEN
        RETURN NEW;
    END IF;

    SELECT status INTO subject_status FROM subjects
        WHERE tenant_id = NEW.tenant_id AND id = NEW.subject_id FOR SHARE;
    IF subject_status IS DISTINCT FROM 'ACTIVE' THEN
        RAISE EXCEPTION 'offering requires active subject'
            USING ERRCODE = '23514', CONSTRAINT = 'offering_subject_unavailable';
    END IF;

    SELECT status, session_id INTO class_status, class_session FROM classes
        WHERE tenant_id = NEW.tenant_id AND id = NEW.class_id FOR SHARE;
    IF class_status IS DISTINCT FROM 'ACTIVE' THEN
        RAISE EXCEPTION 'offering requires active class'
            USING ERRCODE = '23514', CONSTRAINT = 'offering_class_unavailable';
    END IF;

    SELECT session_id INTO term_session FROM academic_terms
        WHERE tenant_id = NEW.tenant_id AND id = NEW.term_id FOR SHARE;
    IF term_session IS DISTINCT FROM class_session THEN
        RAISE EXCEPTION 'offering term and class must share academic session'
            USING ERRCODE = '23514', CONSTRAINT = 'offering_session_mismatch';
    END IF;

    SELECT status INTO session_status FROM academic_sessions
        WHERE tenant_id = NEW.tenant_id AND id = class_session FOR SHARE;
    IF session_status NOT IN ('DRAFT', 'ACTIVE') OR session_status IS NULL THEN
        RAISE EXCEPTION 'offering academic session is unavailable'
            USING ERRCODE = '23514', CONSTRAINT = 'offering_session_unavailable';
    END IF;

    IF NEW.class_arm_id IS NOT NULL THEN
        SELECT status, class_id INTO arm_status, arm_class FROM class_arms
            WHERE tenant_id = NEW.tenant_id AND id = NEW.class_arm_id FOR SHARE;
        IF arm_status IS DISTINCT FROM 'ACTIVE' OR arm_class IS DISTINCT FROM NEW.class_id THEN
            RAISE EXCEPTION 'offering class arm must be active and belong to its class'
                USING ERRCODE = '23514', CONSTRAINT = 'offering_arm_mismatch';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER subject_offerings_guard_mutation
    BEFORE INSERT OR UPDATE OR DELETE ON subject_offerings
    FOR EACH ROW EXECUTE FUNCTION academic_guard_offering_mutation();

CREATE FUNCTION academic_guard_teacher_assignment_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    member_role VARCHAR(40);
    member_status membership_status;
    offering_status VARCHAR(16);
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'teacher assignments cannot be deleted'
            USING ERRCODE = '23514', CONSTRAINT = 'teacher_assignment_delete_forbidden';
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id OR
           NEW.offering_id <> OLD.offering_id OR
           NEW.teacher_membership_id <> OLD.teacher_membership_id THEN
            RAISE EXCEPTION 'teacher assignment identity cannot change'
                USING ERRCODE = '23514', CONSTRAINT = 'teacher_assignment_identity_immutable';
        END IF;
        IF OLD.status = 'INACTIVE' THEN
            RAISE EXCEPTION 'inactive assignments cannot be reactivated'
                USING ERRCODE = '23514', CONSTRAINT = 'teacher_assignment_inactive_immutable';
        END IF;
        NEW.updated_at := CURRENT_TIMESTAMP;
    END IF;

    -- Deactivation must always succeed, including after membership revocation.
    IF NEW.status = 'ACTIVE' THEN
        SELECT role, status INTO member_role, member_status FROM memberships
            WHERE tenant_id = NEW.tenant_id AND id = NEW.teacher_membership_id
            FOR SHARE;
        IF member_role IS DISTINCT FROM 'TEACHER' OR
           member_status IS DISTINCT FROM 'ACTIVE' THEN
            RAISE EXCEPTION 'assignment requires active teacher membership'
                USING ERRCODE = '23514', CONSTRAINT = 'teacher_assignment_member_unavailable';
        END IF;
        SELECT status INTO offering_status FROM subject_offerings
            WHERE tenant_id = NEW.tenant_id AND id = NEW.offering_id FOR SHARE;
        IF offering_status IS DISTINCT FROM 'ACTIVE' THEN
            RAISE EXCEPTION 'assignment requires active subject offering'
                USING ERRCODE = '23514', CONSTRAINT = 'teacher_assignment_offering_unavailable';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER teacher_assignments_guard_mutation
    BEFORE INSERT OR UPDATE OR DELETE ON teacher_assignments
    FOR EACH ROW EXECUTE FUNCTION academic_guard_teacher_assignment_mutation();

INSERT INTO schema_migrations (version) VALUES ('000007_subjects_offerings')
ON CONFLICT (version) DO NOTHING;
