-- Integer hundredths of a percentage point give exact inclusive boundaries.
CREATE FUNCTION valid_assessment_grade_bands(bands jsonb) RETURNS boolean
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
    band jsonb;
    low int;
    high int;
    occupied int4multirange := '{}'::int4multirange;
    span int4range;
    label text;
    labels text[] := ARRAY[]::text[];
BEGIN
    IF jsonb_typeof(bands) IS DISTINCT FROM 'array' THEN
        RETURN false;
    END IF;
    IF jsonb_array_length(bands) > 30 THEN
        RETURN false;
    END IF;
    FOR band IN SELECT value FROM jsonb_array_elements(bands) LOOP
        IF jsonb_typeof(band) IS DISTINCT FROM 'object'
            OR jsonb_typeof(band->'label') IS DISTINCT FROM 'string'
            OR jsonb_typeof(band->'min_bp') IS DISTINCT FROM 'number'
            OR jsonb_typeof(band->'max_bp') IS DISTINCT FROM 'number'
            OR COALESCE(band->>'min_bp','') !~ '^[0-9]{1,5}$'
            OR COALESCE(band->>'max_bp','') !~ '^[0-9]{1,5}$' THEN
            RETURN false;
        END IF;
        label := lower(btrim(band->>'label'));
        low := (band->>'min_bp')::integer;
        high := (band->>'max_bp')::integer;
        IF length(label) NOT BETWEEN 1 AND 40 OR label = ANY(labels) OR low > high OR high > 10000 THEN
            RETURN false;
        END IF;
        span := int4range(low,high,'[]');
        IF occupied && int4multirange(span) THEN
            RETURN false;
        END IF;
        occupied := occupied + int4multirange(span);
        labels := array_append(labels,label);
    END LOOP;
    RETURN true;
END;
$$;

ALTER TABLE assessment_schemes ADD COLUMN grade_bands JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD CONSTRAINT assessment_scheme_grade_bands_valid CHECK (valid_assessment_grade_bands(grade_bands));
