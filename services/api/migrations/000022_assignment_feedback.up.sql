ALTER TABLE assignment_submissions ADD UNIQUE (tenant_id, id);

CREATE TABLE assignment_feedback (
    tenant_id UUID NOT NULL,
    submission_id UUID NOT NULL,
    reviewer_id UUID NOT NULL,
    score INTEGER NOT NULL CHECK (score >= 0),
    comment VARCHAR(4000) NOT NULL CHECK (length(btrim(comment)) > 0),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    returned_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, submission_id),
    FOREIGN KEY (tenant_id, submission_id) REFERENCES assignment_submissions (tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, reviewer_id) REFERENCES memberships (tenant_id, id) ON DELETE RESTRICT
);
