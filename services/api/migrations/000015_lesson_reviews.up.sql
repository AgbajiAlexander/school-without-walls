ALTER TABLE lessons ADD COLUMN content_revision BIGINT NOT NULL DEFAULT 1 CHECK (content_revision > 0);

CREATE TABLE lesson_reviews (
    tenant_id UUID NOT NULL,
    lesson_id UUID NOT NULL,
    lesson_revision BIGINT NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    reviewer_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    approved_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, lesson_id, lesson_revision),
    FOREIGN KEY (tenant_id, lesson_id) REFERENCES lessons(tenant_id, id) ON DELETE RESTRICT,
    CHECK ((reviewer_id IS NULL) = (approved_at IS NULL))
);

CREATE FUNCTION require_lesson_review() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status = 'DRAFT' AND NEW.status = 'PUBLISHED' AND EXISTS (
        SELECT 1 FROM tenants
        WHERE id = OLD.tenant_id AND feature_flags->'lesson_review_required' = 'true'::jsonb
    ) AND NOT EXISTS (
        SELECT 1 FROM lesson_reviews
        WHERE tenant_id = OLD.tenant_id AND lesson_id = OLD.id
          AND lesson_revision = OLD.content_revision AND approved_at IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'lesson revision requires approval' USING ERRCODE = '23514';
    END IF;
    NEW.content_revision := OLD.content_revision;
    IF ROW(NEW.title, NEW.content) IS DISTINCT FROM ROW(OLD.title, OLD.content) THEN
        IF NEW.status = 'PUBLISHED' THEN
            RAISE EXCEPTION 'edit and publication require separate operations' USING ERRCODE = '23514';
        END IF;
        NEW.content_revision := OLD.content_revision + 1;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lesson_review_before_publish
BEFORE UPDATE ON lessons FOR EACH ROW EXECUTE FUNCTION require_lesson_review();
