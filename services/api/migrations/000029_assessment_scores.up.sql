CREATE TABLE assessment_scores (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    enrollment_id UUID NOT NULL,
    offering_id UUID NOT NULL,
    component_id UUID NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    score NUMERIC NOT NULL CHECK (
        score >= 0 AND score <= 9999999.99 AND scale(score) <= 2
    ),
    UNIQUE (tenant_id, enrollment_id, offering_id, component_id),
    FOREIGN KEY (tenant_id, enrollment_id) REFERENCES enrollments (tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, offering_id) REFERENCES subject_offerings (tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, component_id) REFERENCES assessment_components (tenant_id, id) ON DELETE RESTRICT
);

CREATE FUNCTION guard_assessment_score() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    component_max NUMERIC;
BEGIN
    IF TG_OP = 'UPDATE' AND
        ROW(NEW.id, NEW.tenant_id, NEW.enrollment_id, NEW.offering_id, NEW.component_id)
        IS DISTINCT FROM
        ROW(OLD.id, OLD.tenant_id, OLD.enrollment_id, OLD.offering_id, OLD.component_id) THEN
        RAISE EXCEPTION 'assessment score identity is immutable' USING ERRCODE = '23514';
    END IF;

    SELECT component.max_score INTO component_max
    FROM assessment_components component
    JOIN assessment_schemes scheme
        ON scheme.tenant_id = component.tenant_id AND scheme.id = component.scheme_id
    JOIN subject_offerings offering
        ON offering.tenant_id = scheme.tenant_id AND offering.term_id = scheme.term_id
    JOIN classes class
        ON class.tenant_id = offering.tenant_id AND class.id = offering.class_id
    JOIN grade_levels grade
        ON grade.tenant_id = class.tenant_id AND grade.id = class.grade_level_id
    JOIN enrollments placement
        ON placement.tenant_id = offering.tenant_id
        AND placement.term_id = offering.term_id AND placement.class_id = offering.class_id
        AND (offering.class_arm_id IS NULL OR offering.class_arm_id = placement.class_arm_id)
    WHERE component.tenant_id = NEW.tenant_id AND component.id = NEW.component_id
        AND offering.id = NEW.offering_id AND placement.id = NEW.enrollment_id
        AND scheme.status = 'PUBLISHED' AND scheme.stage = grade.stage;

    IF component_max IS NULL OR NEW.score > component_max THEN
        RAISE EXCEPTION 'assessment score is outside its assessment scope or range' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER assessment_score_scope BEFORE INSERT OR UPDATE ON assessment_scores
    FOR EACH ROW EXECUTE FUNCTION guard_assessment_score();
