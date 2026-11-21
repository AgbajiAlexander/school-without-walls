-- Global curated registry. Tenant selections and learner data live elsewhere.
CREATE TABLE curriculum_framework_versions (
    id UUID PRIMARY KEY,
    jurisdiction VARCHAR(12) NOT NULL CHECK (jurisdiction ~ '^[A-Z]{2}(-[A-Z0-9]{1,8})?$'),
    publisher CITEXT NOT NULL CHECK (length(trim(publisher::text)) > 0),
    title CITEXT NOT NULL CHECK (length(trim(title::text)) > 0),
    edition CITEXT NOT NULL CHECK (length(trim(edition::text)) > 0),
    framework_kind VARCHAR(20) NOT NULL CHECK (framework_kind IN ('NIGERIAN_PRIMARY', 'ENRICHMENT', 'DEVELOPMENTAL')),
    effective_from DATE NOT NULL,
    effective_to DATE CHECK (effective_to >= effective_from),
    source_url VARCHAR(2048) NOT NULL CHECK (source_url ~ '^https://[^[:space:]]+$'),
    license_note VARCHAR(2000) NOT NULL CHECK (length(trim(license_note)) > 0),
    status VARCHAR(12) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'PUBLISHED', 'WITHDRAWN')),
    UNIQUE (jurisdiction, publisher, title, edition),
    CHECK (framework_kind <> 'NIGERIAN_PRIMARY' OR jurisdiction = 'NG')
);

CREATE TABLE curriculum_objectives (
    id UUID PRIMARY KEY,
    framework_version_id UUID NOT NULL REFERENCES curriculum_framework_versions(id) ON DELETE RESTRICT,
    stage VARCHAR(20) NOT NULL CHECK (stage IN ('EARLY_YEARS', 'PRIMARY', 'JUNIOR_SECONDARY', 'SENIOR_SECONDARY')),
    grade_band VARCHAR(60) NOT NULL CHECK (length(trim(grade_band)) > 0),
    subject VARCHAR(100) NOT NULL CHECK (length(trim(subject)) > 0),
    strand VARCHAR(100) NOT NULL CHECK (length(trim(strand)) > 0),
    code VARCHAR(64) NOT NULL CHECK (code ~ '^[A-Z0-9][A-Z0-9._-]{0,63}$'),
    objective_text VARCHAR(2000) NOT NULL CHECK (length(trim(objective_text)) > 0),
    UNIQUE (framework_version_id, code)
);

CREATE FUNCTION guard_curriculum_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status <> 'DRAFT' THEN
            RAISE EXCEPTION 'curriculum versions must start as drafts' USING ERRCODE = '23514';
        END IF;
        RETURN NEW;
    END IF;
    IF TG_OP = 'DELETE' THEN
        IF OLD.status <> 'DRAFT' THEN
            RAISE EXCEPTION 'published curriculum history must be retained' USING ERRCODE = '23514';
        END IF;
        RETURN OLD;
    END IF;
    IF OLD.status <> 'DRAFT' THEN
        IF OLD.status <> 'PUBLISHED' OR NEW.status <> 'WITHDRAWN'
            OR (to_jsonb(NEW) - 'status') IS DISTINCT FROM (to_jsonb(OLD) - 'status') THEN
            RAISE EXCEPTION 'published curriculum metadata is immutable' USING ERRCODE = '23514';
        END IF;
    ELSIF NEW.status = 'WITHDRAWN' THEN
        RAISE EXCEPTION 'draft curriculum cannot be withdrawn' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER curriculum_version_lifecycle
BEFORE INSERT OR UPDATE OR DELETE ON curriculum_framework_versions
FOR EACH ROW EXECUTE FUNCTION guard_curriculum_version();

CREATE FUNCTION guard_curriculum_objective() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    version_id UUID;
    version_status VARCHAR(12);
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.framework_version_id <> OLD.framework_version_id THEN
        RAISE EXCEPTION 'curriculum objectives cannot move across editions' USING ERRCODE = '23514';
    END IF;
    IF TG_OP = 'DELETE' THEN
        version_id := OLD.framework_version_id;
    ELSE
        version_id := NEW.framework_version_id;
    END IF;

    -- Serialize objective changes against publication and withdrawal.
    SELECT status INTO version_status FROM curriculum_framework_versions
    WHERE id = version_id FOR SHARE;
    IF version_status IS DISTINCT FROM 'DRAFT' THEN
        RAISE EXCEPTION 'curriculum objectives require a draft edition' USING ERRCODE = '23514';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER curriculum_objective_lifecycle
BEFORE INSERT OR UPDATE OR DELETE ON curriculum_objectives
FOR EACH ROW EXECUTE FUNCTION guard_curriculum_objective();
