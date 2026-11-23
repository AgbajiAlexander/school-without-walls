CREATE TABLE assignment_submissions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    assignment_id UUID NOT NULL,
    student_id UUID NOT NULL,
    request_id UUID NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    content VARCHAR(24000) NOT NULL CHECK (length(btrim(content)) > 0),
    submitted_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (tenant_id, assignment_id) REFERENCES assignments (tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, student_id) REFERENCES students (tenant_id, id) ON DELETE RESTRICT,
    UNIQUE (tenant_id, assignment_id, student_id, request_id),
    UNIQUE (tenant_id, assignment_id, student_id, revision)
);
