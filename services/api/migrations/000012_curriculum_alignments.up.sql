-- SWW-006: global, human-reviewed links from Nigerian objectives to enrichment.
CREATE TABLE curriculum_objective_alignments (
    id UUID PRIMARY KEY,
    primary_objective_id UUID NOT NULL REFERENCES curriculum_objectives(id) ON DELETE RESTRICT,
    related_objective_id UUID NOT NULL REFERENCES curriculum_objectives(id) ON DELETE RESTRICT,
    relation VARCHAR(20) NOT NULL CHECK (relation IN ('EQUIVALENT', 'COMPLEMENTARY', 'EXTENDS', 'DIFFERENT')),
    review_status VARCHAR(16) NOT NULL DEFAULT 'DRAFT' CHECK (review_status IN ('DRAFT', 'HUMAN_REVIEWED')),
    reviewer_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    UNIQUE (primary_objective_id, related_objective_id),
    CHECK (primary_objective_id <> related_objective_id),
    CHECK (
        (review_status = 'DRAFT' AND reviewer_id IS NULL AND reviewed_at IS NULL)
        OR (review_status = 'HUMAN_REVIEWED' AND reviewer_id IS NOT NULL AND reviewed_at IS NOT NULL)
    )
);

CREATE INDEX curriculum_alignments_related_idx ON curriculum_objective_alignments (related_objective_id);

CREATE FUNCTION guard_curriculum_alignment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP <> 'INSERT' AND OLD.review_status = 'HUMAN_REVIEWED' THEN
        RAISE EXCEPTION 'reviewed curriculum alignments are immutable' USING ERRCODE = '23514';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    IF TG_OP = 'INSERT' AND NEW.review_status <> 'DRAFT' THEN
        RAISE EXCEPTION 'curriculum alignments must start as drafts' USING ERRCODE = '23514';
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM curriculum_objectives p
        JOIN curriculum_framework_versions pf ON pf.id = p.framework_version_id
        CROSS JOIN curriculum_objectives r
        JOIN curriculum_framework_versions rf ON rf.id = r.framework_version_id
        WHERE p.id = NEW.primary_objective_id AND r.id = NEW.related_objective_id
          AND pf.framework_kind = 'NIGERIAN_PRIMARY' AND rf.framework_kind = 'ENRICHMENT'
          AND (NEW.review_status = 'DRAFT' OR (pf.status = 'PUBLISHED' AND rf.status = 'PUBLISHED'))
        FOR SHARE OF pf, rf
    ) THEN
        RAISE EXCEPTION 'alignment requires Nigerian and enrichment objectives; review requires published editions'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.review_status = 'HUMAN_REVIEWED' THEN
        -- Material edits require a separate draft write before review.
        IF ROW(NEW.id, NEW.primary_objective_id, NEW.related_objective_id, NEW.relation)
            IS DISTINCT FROM ROW(OLD.id, OLD.primary_objective_id, OLD.related_objective_id, OLD.relation) THEN
            RAISE EXCEPTION 'cannot edit an alignment while reviewing it' USING ERRCODE = '23514';
        END IF;
        NEW.reviewed_at := clock_timestamp();
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER curriculum_alignment_lifecycle
BEFORE INSERT OR UPDATE OR DELETE ON curriculum_objective_alignments
FOR EACH ROW EXECUTE FUNCTION guard_curriculum_alignment();

-- Retain the historical review when either edition is withdrawn.
CREATE VIEW curriculum_verified_alignments AS
SELECT a.id, a.primary_objective_id, a.related_objective_id, a.relation, a.reviewed_at
FROM curriculum_objective_alignments a
JOIN curriculum_objectives p ON p.id = a.primary_objective_id
JOIN curriculum_framework_versions pf ON pf.id = p.framework_version_id
JOIN curriculum_objectives r ON r.id = a.related_objective_id
JOIN curriculum_framework_versions rf ON rf.id = r.framework_version_id
WHERE a.review_status = 'HUMAN_REVIEWED' AND pf.status = 'PUBLISHED' AND rf.status = 'PUBLISHED';
