ALTER TABLE assignment_submissions ALTER COLUMN content DROP NOT NULL;

ALTER TABLE assignment_submissions ADD COLUMN file_id UUID UNIQUE,
    ADD COLUMN file_checksum TEXT,
    ADD CHECK (
        (content IS NOT NULL AND file_id IS NULL AND file_checksum IS NULL)
        OR (content IS NULL AND file_id IS NOT NULL AND file_checksum IS NOT NULL
            AND file_checksum ~ '^[A-Za-z0-9+/]{43}=$')
    );
