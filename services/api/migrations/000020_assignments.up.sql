CREATE TABLE assignments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    lesson_id UUID NOT NULL,
    title VARCHAR(160) NOT NULL CHECK (length(btrim(title)) > 0),
    instructions VARCHAR(24000) NOT NULL CHECK (length(btrim(instructions)) > 0),
    due_at TIMESTAMPTZ NOT NULL CHECK (isfinite(due_at)),
    max_score INTEGER NOT NULL CHECK (max_score > 0),
    submission_type TEXT NOT NULL CHECK (submission_type IN ('TEXT', 'FILE')),
    allow_resubmission BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'OPEN', 'CLOSED')),
    opened_at TIMESTAMPTZ,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, lesson_id) REFERENCES lessons (tenant_id, id) ON DELETE RESTRICT,
    CHECK ((status = 'DRAFT') = (opened_at IS NULL)),
    CHECK (opened_at IS NULL OR opened_at < due_at)
);

CREATE INDEX assignments_lesson_status ON assignments (tenant_id, lesson_id, status, id);

CREATE FUNCTION guard_assignment_lifecycle() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status <> 'DRAFT' OR NEW.opened_at IS NOT NULL THEN
            RAISE EXCEPTION 'new assignments must start as drafts' USING ERRCODE = '23514';
        END IF;
        RETURN NEW;
    END IF;

    IF ROW(NEW.id, NEW.tenant_id, NEW.lesson_id) IS DISTINCT FROM ROW(OLD.id, OLD.tenant_id, OLD.lesson_id) THEN
        RAISE EXCEPTION 'assignment identity and lesson are immutable' USING ERRCODE = '23514';
    END IF;
    IF NEW.opened_at IS DISTINCT FROM OLD.opened_at THEN
        RAISE EXCEPTION 'assignment opening time is database controlled' USING ERRCODE = '23514';
    END IF;

    IF OLD.status = 'DRAFT' AND NEW.status = 'OPEN' THEN
        PERFORM 1 FROM lessons
        WHERE tenant_id = NEW.tenant_id AND id = NEW.lesson_id AND status = 'PUBLISHED'
        FOR SHARE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'an assignment requires a published lesson before opening' USING ERRCODE = '23514';
        END IF;
        NEW.opened_at := clock_timestamp();
        IF NEW.due_at <= NEW.opened_at THEN
            RAISE EXCEPTION 'assignment deadline has passed' USING ERRCODE = '23514';
        END IF;
    ELSIF OLD.status = 'OPEN' AND NEW.status = 'CLOSED' THEN
        IF ROW(NEW.title, NEW.instructions, NEW.due_at, NEW.max_score, NEW.submission_type, NEW.allow_resubmission)
            IS DISTINCT FROM ROW(OLD.title, OLD.instructions, OLD.due_at, OLD.max_score, OLD.submission_type, OLD.allow_resubmission) THEN
            RAISE EXCEPTION 'open assignment settings are immutable' USING ERRCODE = '23514';
        END IF;
    ELSIF OLD.status <> 'DRAFT' OR NEW.status <> 'DRAFT' THEN
        RAISE EXCEPTION 'invalid assignment state transition' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER assignments_lifecycle BEFORE INSERT OR UPDATE ON assignments
FOR EACH ROW EXECUTE FUNCTION guard_assignment_lifecycle();
