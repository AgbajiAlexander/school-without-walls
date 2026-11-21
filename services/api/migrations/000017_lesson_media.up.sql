-- Private object keys are derived by the application from tenant ID and media ID.
-- Only the scanner can make a quarantined object eligible for attachment.
CREATE TABLE lesson_media (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    lesson_id UUID NOT NULL,
    media_type VARCHAR(24) NOT NULL CHECK (media_type IN ('application/pdf','image/jpeg','image/png')),
    size_bytes INTEGER NOT NULL CHECK (size_bytes BETWEEN 1 AND 8388608),
    status VARCHAR(12) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','READY','REJECTED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    scanned_at TIMESTAMPTZ,
    FOREIGN KEY (tenant_id,lesson_id) REFERENCES lessons(tenant_id,id) ON DELETE RESTRICT,
    UNIQUE (tenant_id,lesson_id,id),
    CHECK ((status = 'PENDING') = (scanned_at IS NULL))
);

CREATE INDEX lesson_media_pending ON lesson_media (created_at,id) WHERE status='PENDING';

ALTER TABLE lessons ADD COLUMN media_id UUID,
    ADD FOREIGN KEY (tenant_id,id,media_id) REFERENCES lesson_media(tenant_id,lesson_id,id) ON DELETE RESTRICT;

CREATE FUNCTION guard_lesson_media_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status <> 'PENDING' OR NEW.scanned_at IS NOT NULL THEN
            RAISE EXCEPTION 'new lesson media must start in quarantine' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    IF ROW(NEW.id,NEW.tenant_id,NEW.lesson_id,NEW.media_type,NEW.size_bytes,NEW.created_at)
        IS DISTINCT FROM ROW(OLD.id,OLD.tenant_id,OLD.lesson_id,OLD.media_type,OLD.size_bytes,OLD.created_at)
        OR NOT (
            (OLD.status='PENDING' AND NEW.status IN ('READY','REJECTED'))
            OR (OLD.status='READY' AND NEW.status='REJECTED')
        ) THEN
        RAISE EXCEPTION 'invalid media scan transition' USING ERRCODE='23514';
    END IF;
    NEW.scanned_at := clock_timestamp();
    RETURN NEW;
END;
$$;

CREATE TRIGGER lesson_media_scan_state BEFORE INSERT OR UPDATE ON lesson_media
FOR EACH ROW EXECUTE FUNCTION guard_lesson_media_state();

CREATE FUNCTION guard_lesson_media_attachment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='UPDATE' THEN
        IF OLD.status<>'DRAFT' AND NEW.media_id IS DISTINCT FROM OLD.media_id THEN
            RAISE EXCEPTION 'published lesson media reference is immutable' USING ERRCODE='23514';
        END IF;
        IF NEW.status<>'PUBLISHED' AND NEW.media_id IS NOT DISTINCT FROM OLD.media_id THEN
            RETURN NEW;
        END IF;
    END IF;
    IF NEW.media_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM lesson_media
        WHERE tenant_id=NEW.tenant_id AND lesson_id=NEW.id AND id=NEW.media_id AND status='READY'
        FOR SHARE
    ) THEN
        RAISE EXCEPTION 'lesson media has not passed scanning for this lesson' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lesson_media_attachment BEFORE INSERT OR UPDATE ON lessons
FOR EACH ROW EXECUTE FUNCTION guard_lesson_media_attachment();

CREATE OR REPLACE FUNCTION require_lesson_review() RETURNS trigger LANGUAGE plpgsql AS $$
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
    IF ROW(NEW.title, NEW.content, NEW.enrichment_alignment_ids, NEW.media_id) IS DISTINCT FROM ROW(OLD.title, OLD.content, OLD.enrichment_alignment_ids, OLD.media_id) THEN
        IF NEW.status = 'PUBLISHED' THEN
            RAISE EXCEPTION 'edit and publication require separate operations' USING ERRCODE = '23514';
        END IF;
        NEW.content_revision := OLD.content_revision + 1;
    END IF;
    RETURN NEW;
END;
$$;
