CREATE TABLE assessment_schemes (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    term_id UUID NOT NULL,
    stage TEXT NOT NULL CHECK (stage IN ('PRIMARY','JUNIOR_SECONDARY','SENIOR_SECONDARY')),
    version INTEGER NOT NULL CHECK (version > 0),
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','PUBLISHED')),
    published_at TIMESTAMPTZ,
    components_revision BIGINT NOT NULL DEFAULT 0 CHECK (components_revision >= 0),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, term_id, stage, version),
    FOREIGN KEY (tenant_id, term_id) REFERENCES academic_terms (tenant_id, id) ON DELETE RESTRICT,
    CHECK ((status = 'PUBLISHED') = (published_at IS NOT NULL))
);

CREATE TABLE assessment_components (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    scheme_id UUID NOT NULL,
    name CITEXT NOT NULL CHECK (length(btrim(name::text)) BETWEEN 1 AND 100),
    max_score NUMERIC(9,2) NOT NULL CHECK (max_score > 0 AND max_score <> 'NaN'::numeric),
    weight NUMERIC(5,2) NOT NULL CHECK (weight > 0 AND weight <= 100),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, scheme_id, name),
    FOREIGN KEY (tenant_id, scheme_id) REFERENCES assessment_schemes (tenant_id, id) ON DELETE RESTRICT
);

CREATE FUNCTION guard_assessment_scheme() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status <> 'DRAFT' OR NEW.published_at IS NOT NULL THEN
            RAISE EXCEPTION 'assessment schemes must start as drafts' USING ERRCODE = '23514';
        END IF;
        RETURN NEW;
    END IF;
    IF OLD.status = 'PUBLISHED' THEN
        RAISE EXCEPTION 'published assessment schemes are immutable' USING ERRCODE = '23514';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    IF ROW(NEW.id,NEW.tenant_id,NEW.term_id,NEW.stage,NEW.version,NEW.published_at)
        IS DISTINCT FROM ROW(OLD.id,OLD.tenant_id,OLD.term_id,OLD.stage,OLD.version,OLD.published_at) THEN
        RAISE EXCEPTION 'assessment scheme identity and publication time are server controlled' USING ERRCODE = '23514';
    END IF;
    IF NEW.status = 'PUBLISHED' THEN
        IF (SELECT sum(weight) FROM assessment_components WHERE tenant_id=OLD.tenant_id AND scheme_id=OLD.id)
            IS DISTINCT FROM 100.00::numeric THEN
            RAISE EXCEPTION 'assessment weights must total 100 percent' USING ERRCODE = '23514';
        END IF;
        NEW.published_at := clock_timestamp();
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION guard_assessment_component() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target assessment_components;
    scheme_status text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        target := OLD;
    ELSE
        target := NEW;
    END IF;
    IF TG_OP = 'UPDATE' AND ROW(NEW.id,NEW.tenant_id,NEW.scheme_id)
        IS DISTINCT FROM ROW(OLD.id,OLD.tenant_id,OLD.scheme_id) THEN
        RAISE EXCEPTION 'assessment component identity is immutable' USING ERRCODE = '23514';
    END IF;

    -- The same parent lock serializes component edits with publication.
    SELECT status INTO scheme_status FROM assessment_schemes
        WHERE tenant_id=target.tenant_id AND id=target.scheme_id FOR NO KEY UPDATE;
    IF scheme_status = 'PUBLISHED' THEN
        RAISE EXCEPTION 'published assessment components are immutable' USING ERRCODE = '23514';
    END IF;
    -- Change the parent row version too. Older Repeatable Read publication
    -- snapshots must fail serialization instead of summing stale components.
    UPDATE assessment_schemes SET components_revision=components_revision+1
        WHERE tenant_id=target.tenant_id AND id=target.scheme_id;
    RETURN target;
END;
$$;

CREATE TRIGGER assessment_scheme_lifecycle BEFORE INSERT OR UPDATE OR DELETE ON assessment_schemes
    FOR EACH ROW EXECUTE FUNCTION guard_assessment_scheme();
CREATE TRIGGER assessment_component_lifecycle BEFORE INSERT OR UPDATE OR DELETE ON assessment_components
    FOR EACH ROW EXECUTE FUNCTION guard_assessment_component();
