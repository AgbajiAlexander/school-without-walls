-- SWW-007: text-first lessons pinned to the original curriculum selection.
CREATE TABLE lessons (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    offering_id UUID NOT NULL,
    author_membership_id UUID NOT NULL,
    curriculum_selection_id UUID NOT NULL,
    primary_objective_id UUID NOT NULL REFERENCES curriculum_objectives(id) ON DELETE RESTRICT,
    title VARCHAR(160) NOT NULL CHECK (length(trim(title)) > 0),
    content VARCHAR(24000) NOT NULL CHECK (length(trim(content)) > 0),
    status VARCHAR(12) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'PUBLISHED', 'ARCHIVED')),
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, offering_id) REFERENCES subject_offerings(tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, author_membership_id) REFERENCES memberships(tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, curriculum_selection_id) REFERENCES school_curriculum_selections(tenant_id, id) ON DELETE RESTRICT,
    CHECK ((status = 'DRAFT') = (published_at IS NULL))
);

CREATE INDEX lessons_offering_status ON lessons (tenant_id, offering_id, status, id);
CREATE INDEX lessons_author_status ON lessons (tenant_id, author_membership_id, status, id);

CREATE FUNCTION guard_lesson_lifecycle() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status <> 'DRAFT' THEN
            RAISE EXCEPTION 'published lesson history must be retained' USING ERRCODE = '23514';
        END IF;
        RETURN OLD;
    END IF;
    IF TG_OP = 'INSERT' AND NEW.status <> 'DRAFT' THEN
        RAISE EXCEPTION 'lessons must start as drafts' USING ERRCODE = '23514';
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF ROW(NEW.id, NEW.tenant_id, NEW.offering_id, NEW.author_membership_id, NEW.curriculum_selection_id,
               NEW.primary_objective_id, NEW.created_at)
           IS DISTINCT FROM ROW(OLD.id, OLD.tenant_id, OLD.offering_id, OLD.author_membership_id, OLD.curriculum_selection_id,
               OLD.primary_objective_id, OLD.created_at) THEN
            RAISE EXCEPTION 'lesson identity and curriculum references are immutable' USING ERRCODE = '23514';
        END IF;
        IF OLD.status = 'ARCHIVED'
            OR (OLD.status = 'DRAFT' AND NEW.status = 'ARCHIVED')
            OR (OLD.status = 'PUBLISHED' AND (
                NEW.status <> 'ARCHIVED' OR ROW(NEW.title, NEW.content, NEW.published_at)
                    IS DISTINCT FROM ROW(OLD.title, OLD.content, OLD.published_at)
            )) THEN
            RAISE EXCEPTION 'invalid lesson state change' USING ERRCODE = '23514';
        END IF;
    END IF;

    IF TG_OP = 'INSERT' OR NEW.status = 'PUBLISHED' THEN
        IF NOT EXISTS (
            SELECT 1 FROM subject_offerings o
            JOIN academic_terms term ON term.tenant_id=o.tenant_id AND term.id=o.term_id
            JOIN academic_sessions session ON session.tenant_id=term.tenant_id AND session.id=term.session_id
            JOIN classes c ON c.tenant_id=o.tenant_id AND c.id=o.class_id AND c.session_id=term.session_id
            JOIN grade_levels g ON g.tenant_id=c.tenant_id AND g.id=c.grade_level_id
            JOIN school_curriculum_selections s ON s.tenant_id=o.tenant_id AND s.id=NEW.curriculum_selection_id
                AND s.session_id=term.session_id AND s.stage=g.stage AND s.curriculum_role='PRIMARY'
            JOIN curriculum_framework_versions f ON f.id=s.framework_version_id AND f.status='PUBLISHED'
            JOIN curriculum_objectives p ON p.id=NEW.primary_objective_id
                AND p.framework_version_id=f.id AND p.stage=s.stage
            WHERE o.tenant_id=NEW.tenant_id AND o.id=NEW.offering_id AND o.status='ACTIVE'
              AND c.status='ACTIVE' AND g.status='ACTIVE' AND session.status <> 'ARCHIVED'
              AND (NEW.status <> 'PUBLISHED' OR session.status = 'ACTIVE')
              AND (
                  TG_OP <> 'INSERT' OR EXISTS (
                      SELECT 1 FROM school_current_curriculum_selections current
                      WHERE current.tenant_id=s.tenant_id AND current.id=s.id
                  )
              )
            FOR SHARE OF f
        ) THEN
            RAISE EXCEPTION 'lesson requires an eligible primary curriculum selection and offering'
                USING ERRCODE = '23514';
        END IF;
    END IF;
    IF NEW.status = 'PUBLISHED' THEN
        NEW.published_at := clock_timestamp();
    END IF;
    NEW.updated_at := clock_timestamp();
    RETURN NEW;
END;
$$;

CREATE TRIGGER lessons_lifecycle
BEFORE INSERT OR UPDATE OR DELETE ON lessons
FOR EACH ROW EXECUTE FUNCTION guard_lesson_lifecycle();
