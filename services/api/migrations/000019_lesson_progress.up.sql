CREATE TABLE lesson_progress (
    tenant_id UUID NOT NULL,
    lesson_id UUID NOT NULL,
    student_id UUID NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, lesson_id, student_id),
    FOREIGN KEY (tenant_id, lesson_id) REFERENCES lessons(tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, student_id) REFERENCES students(tenant_id, id) ON DELETE RESTRICT,
    CHECK (completed_at IS NULL OR completed_at >= started_at)
);
