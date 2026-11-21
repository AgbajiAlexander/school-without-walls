ALTER TABLE lessons ADD COLUMN enrichment_alignment_ids UUID[] NOT NULL DEFAULT '{}'
    CHECK (cardinality(enrichment_alignment_ids) <= 10 AND array_position(enrichment_alignment_ids, NULL) IS NULL);

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
    IF ROW(NEW.title, NEW.content, NEW.enrichment_alignment_ids) IS DISTINCT FROM ROW(OLD.title, OLD.content, OLD.enrichment_alignment_ids) THEN
        IF NEW.status = 'PUBLISHED' THEN
            RAISE EXCEPTION 'edit and publication require separate operations' USING ERRCODE = '23514';
        END IF;
        NEW.content_revision := OLD.content_revision + 1;
    END IF;
    RETURN NEW;
END;
$$;


CREATE FUNCTION guard_lesson_enrichment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    eligible INTEGER;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF OLD.status <> 'DRAFT' AND NEW.enrichment_alignment_ids IS DISTINCT FROM OLD.enrichment_alignment_ids THEN
            RAISE EXCEPTION 'published lesson enrichment is immutable' USING ERRCODE='23514';
        END IF;
        IF NEW.status <> 'PUBLISHED' AND NEW.enrichment_alignment_ids IS NOT DISTINCT FROM OLD.enrichment_alignment_ids THEN
            RETURN NEW;
        END IF;
    END IF;
    IF cardinality(NEW.enrichment_alignment_ids) = 0 THEN
        RETURN NEW;
    END IF;

    SELECT count(*) INTO eligible FROM (
        SELECT a.id FROM curriculum_objective_alignments a
        JOIN curriculum_objectives p ON p.id=a.primary_objective_id
        JOIN curriculum_framework_versions pf ON pf.id=p.framework_version_id AND pf.status='PUBLISHED'
        JOIN curriculum_objectives r ON r.id=a.related_objective_id AND r.stage=p.stage
        JOIN curriculum_framework_versions rf ON rf.id=r.framework_version_id AND rf.status='PUBLISHED'
        JOIN school_curriculum_selections primary_choice ON primary_choice.tenant_id=NEW.tenant_id
            AND primary_choice.id=NEW.curriculum_selection_id AND primary_choice.framework_version_id=pf.id
        JOIN school_current_curriculum_selections extension ON extension.tenant_id=primary_choice.tenant_id
            AND extension.session_id=primary_choice.session_id AND extension.stage=p.stage
            AND extension.curriculum_role='ENRICHMENT' AND extension.framework_version_id=rf.id
        WHERE a.id=ANY(NEW.enrichment_alignment_ids) AND a.primary_objective_id=NEW.primary_objective_id
            AND a.review_status='HUMAN_REVIEWED'
        FOR SHARE OF pf, rf
    ) mappings;
    IF eligible <> cardinality(NEW.enrichment_alignment_ids) THEN
        RAISE EXCEPTION 'lesson enrichment requires distinct reviewed, published mappings for the selected stage and edition'
            USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER lesson_enrichment_guard
BEFORE INSERT OR UPDATE ON lessons FOR EACH ROW EXECUTE FUNCTION guard_lesson_enrichment();
