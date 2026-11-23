ALTER TABLE enrollments ADD CONSTRAINT enrollments_attendance_placement_unique
    UNIQUE (tenant_id, id, student_id, session_id, term_id, class_id);

CREATE TABLE attendance_records (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    enrollment_id UUID NOT NULL,
    student_id UUID NOT NULL,
    session_id UUID NOT NULL,
    term_id UUID NOT NULL,
    class_id UUID NOT NULL,
    on_date DATE NOT NULL CHECK (isfinite(on_date)),
    mode TEXT NOT NULL CHECK (mode IN ('ON_SITE', 'REMOTE')),
    status TEXT NOT NULL CHECK (status IN ('PRESENT', 'ABSENT', 'LATE', 'EXCUSED')),
    recorded_by UUID NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (tenant_id, student_id, session_id, class_id, on_date),
    FOREIGN KEY (tenant_id, enrollment_id, student_id, session_id, term_id, class_id)
        REFERENCES enrollments (tenant_id, id, student_id, session_id, term_id, class_id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, recorded_by) REFERENCES memberships (tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX attendance_records_class_day ON attendance_records (tenant_id, session_id, class_id, on_date);

CREATE INDEX enrollments_attendance_term_history ON enrollments (tenant_id, term_id, student_id, created_at DESC);
