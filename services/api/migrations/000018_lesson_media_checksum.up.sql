ALTER TABLE lesson_media ADD COLUMN checksum_sha256 VARCHAR(44)
    CHECK (checksum_sha256 ~ '^[A-Za-z0-9+/]{43}=$');

CREATE OR REPLACE FUNCTION guard_lesson_media_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status <> 'PENDING' OR NEW.scanned_at IS NOT NULL THEN
            RAISE EXCEPTION 'new lesson media must start in quarantine' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    IF ROW(NEW.id,NEW.tenant_id,NEW.lesson_id,NEW.media_type,NEW.size_bytes,NEW.created_at,NEW.checksum_sha256)
        IS DISTINCT FROM ROW(OLD.id,OLD.tenant_id,OLD.lesson_id,OLD.media_type,OLD.size_bytes,OLD.created_at,OLD.checksum_sha256)
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
