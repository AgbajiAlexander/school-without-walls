-- SWW-004A academic sessions and terms.

CREATE TABLE academic_sessions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    name VARCHAR(32) NOT NULL,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'DRAFT',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, name),
    CONSTRAINT academic_sessions_name_nonempty CHECK (length(trim(name)) BETWEEN 1 AND 32),
    CONSTRAINT academic_sessions_date_range CHECK (starts_on <= ends_on),
    CONSTRAINT academic_sessions_status CHECK (status IN ('DRAFT', 'ACTIVE', 'ARCHIVED'))
);

CREATE UNIQUE INDEX academic_sessions_one_active_per_tenant
    ON academic_sessions (tenant_id)
    WHERE status = 'ACTIVE';

CREATE INDEX academic_sessions_tenant_schedule
    ON academic_sessions (tenant_id, starts_on DESC, name);

CREATE TABLE academic_terms (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    session_id UUID NOT NULL,
    sequence SMALLINT NOT NULL,
    name VARCHAR(80) NOT NULL,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, session_id, sequence),
    CONSTRAINT academic_terms_session_fk
        FOREIGN KEY (tenant_id, session_id)
        REFERENCES academic_sessions (tenant_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT academic_terms_sequence CHECK (sequence BETWEEN 1 AND 3),
    CONSTRAINT academic_terms_name_nonempty CHECK (length(trim(name)) BETWEEN 1 AND 80),
    CONSTRAINT academic_terms_date_range CHECK (starts_on <= ends_on)
);

CREATE FUNCTION academic_guard_session_update() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    term_count INTEGER;
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status <> 'DRAFT' THEN
            RAISE EXCEPTION 'new academic sessions must start in draft'
                USING ERRCODE = '23514', CONSTRAINT = 'academic_session_transition';
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id THEN
        RAISE EXCEPTION 'academic session identity is immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'academic_session_identity_immutable';
    END IF;

    IF OLD.status = 'ARCHIVED' THEN
        RAISE EXCEPTION 'archived academic sessions are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'academic_session_archived_immutable';
    END IF;

    IF OLD.status = 'ACTIVE' AND NEW.status NOT IN ('ACTIVE', 'ARCHIVED') THEN
        RAISE EXCEPTION 'invalid academic session transition'
            USING ERRCODE = '23514', CONSTRAINT = 'academic_session_transition';
    END IF;

    IF OLD.status = 'DRAFT' AND NEW.status NOT IN ('DRAFT', 'ACTIVE', 'ARCHIVED') THEN
        RAISE EXCEPTION 'invalid academic session transition'
            USING ERRCODE = '23514', CONSTRAINT = 'academic_session_transition';
    END IF;

    IF EXISTS (
        SELECT 1 FROM academic_terms t
        WHERE t.tenant_id = NEW.tenant_id AND t.session_id = NEW.id
            AND (t.starts_on < NEW.starts_on OR t.ends_on > NEW.ends_on)
    ) THEN
        RAISE EXCEPTION 'academic session dates exclude a configured term'
            USING ERRCODE = '23514', CONSTRAINT = 'academic_session_term_bounds';
    END IF;

    IF NEW.status = 'ACTIVE' AND OLD.status <> 'ACTIVE' THEN
        SELECT count(*) INTO term_count
        FROM academic_terms t
        WHERE t.tenant_id = NEW.tenant_id AND t.session_id = NEW.id;

        IF term_count <> 3 THEN
            RAISE EXCEPTION 'exactly three terms are required to activate an academic session'
                USING ERRCODE = '23514', CONSTRAINT = 'academic_session_terms_required';
        END IF;
    END IF;

    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;

CREATE TRIGGER academic_sessions_guard_update
    BEFORE INSERT OR UPDATE ON academic_sessions
    FOR EACH ROW EXECUTE FUNCTION academic_guard_session_update();

CREATE FUNCTION academic_guard_term_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    session_start DATE;
    session_end DATE;
    session_status VARCHAR(16);
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'academic terms cannot be deleted'
            USING ERRCODE = '23514', CONSTRAINT = 'academic_term_delete_forbidden';
    END IF;

    IF TG_OP = 'UPDATE' AND (
        NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id OR NEW.session_id <> OLD.session_id
    ) THEN
        RAISE EXCEPTION 'academic term identity is immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'academic_term_identity_immutable';
    END IF;

    SELECT s.starts_on, s.ends_on, s.status
    INTO session_start, session_end, session_status
    FROM academic_sessions s
    WHERE s.tenant_id = NEW.tenant_id AND s.id = NEW.session_id
    FOR UPDATE;

    IF session_status = 'ARCHIVED' THEN
        RAISE EXCEPTION 'terms of archived academic sessions are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'academic_term_archived_immutable';
    END IF;

    IF NEW.starts_on < session_start OR NEW.ends_on > session_end THEN
        RAISE EXCEPTION 'term dates must fall within the academic session'
            USING ERRCODE = '23514', CONSTRAINT = 'academic_term_outside_session';
    END IF;

    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;

CREATE TRIGGER academic_terms_guard_mutation
    BEFORE INSERT OR UPDATE OR DELETE ON academic_terms
    FOR EACH ROW EXECUTE FUNCTION academic_guard_term_mutation();

CREATE FUNCTION academic_check_term_set() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM academic_terms left_term
        JOIN academic_terms right_term
          ON right_term.tenant_id = left_term.tenant_id
         AND right_term.session_id = left_term.session_id
         AND right_term.id > left_term.id
         AND right_term.starts_on <= left_term.ends_on
         AND right_term.ends_on >= left_term.starts_on
        WHERE left_term.tenant_id = NEW.tenant_id
          AND left_term.session_id = NEW.session_id
    ) THEN
        RAISE EXCEPTION 'academic term dates overlap another term'
            USING ERRCODE = '23P01', CONSTRAINT = 'academic_term_no_overlap';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM academic_terms earlier
        JOIN academic_terms later
          ON later.tenant_id = earlier.tenant_id
         AND later.session_id = earlier.session_id
         AND later.sequence > earlier.sequence
         AND later.starts_on <= earlier.starts_on
        WHERE earlier.tenant_id = NEW.tenant_id
          AND earlier.session_id = NEW.session_id
    ) THEN
        RAISE EXCEPTION 'academic term dates must follow their configured sequence'
            USING ERRCODE = '23514', CONSTRAINT = 'academic_term_sequence_order';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER academic_terms_validate_set
    AFTER INSERT OR UPDATE ON academic_terms
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION academic_check_term_set();

INSERT INTO schema_migrations (version) VALUES ('000005_academics')
ON CONFLICT (version) DO NOTHING;
