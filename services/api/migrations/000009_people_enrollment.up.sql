-- SWW-005: tenant-owned student, guardian, and enrollment records.
-- 000008 is reserved for Staff Invitation delivery on the separate feature branch.
-- Global user identities and role grants remain owned by auth/memberships.

CREATE TABLE students (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id UUID,
    membership_role VARCHAR(40) GENERATED ALWAYS AS ('STUDENT') STORED,
    admission_no CITEXT NOT NULL CHECK (length(trim(admission_no::text)) BETWEEN 1 AND 40),
    display_name VARCHAR(150) NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 150),
    stage VARCHAR(20) NOT NULL CHECK (stage IN ('EARLY_YEARS', 'PRIMARY', 'JUNIOR_SECONDARY', 'SENIOR_SECONDARY')),
    birth_date DATE,
    status VARCHAR(16) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, admission_no),
    UNIQUE (tenant_id, user_id),
    FOREIGN KEY (tenant_id, user_id, membership_role)
        REFERENCES memberships (tenant_id, user_id, role) ON DELETE RESTRICT
);

CREATE TABLE guardians (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id UUID,
    membership_role VARCHAR(40) GENERATED ALWAYS AS ('PARENT') STORED,
    display_name VARCHAR(150) NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 150),
    contact_email CITEXT CHECK (length(contact_email::text) BETWEEN 1 AND 254),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, user_id),
    FOREIGN KEY (tenant_id, user_id, membership_role)
        REFERENCES memberships (tenant_id, user_id, role) ON DELETE RESTRICT
);

CREATE TABLE student_guardians (
    tenant_id UUID NOT NULL,
    student_id UUID NOT NULL,
    guardian_id UUID NOT NULL,
    relationship VARCHAR(40) NOT NULL CHECK (length(trim(relationship)) BETWEEN 1 AND 40),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, student_id, guardian_id),
    FOREIGN KEY (tenant_id, student_id) REFERENCES students (tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, guardian_id) REFERENCES guardians (tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX student_guardians_active_by_guardian
    ON student_guardians (tenant_id, guardian_id, student_id) WHERE active;

ALTER TABLE academic_terms ADD CONSTRAINT academic_terms_tenant_session_id_unique UNIQUE (tenant_id, session_id, id);
ALTER TABLE classes ADD CONSTRAINT classes_tenant_session_id_unique UNIQUE (tenant_id, session_id, id);
ALTER TABLE class_arms ADD CONSTRAINT class_arms_tenant_class_id_unique UNIQUE (tenant_id, class_id, id);

CREATE TABLE enrollments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    student_id UUID NOT NULL,
    session_id UUID NOT NULL,
    term_id UUID NOT NULL,
    class_id UUID NOT NULL,
    class_arm_id UUID,
    status VARCHAR(16) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ENDED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at TIMESTAMPTZ,
    UNIQUE (tenant_id, id),
    CHECK ((status = 'ENDED') = (ended_at IS NOT NULL)),
    CHECK (ended_at IS NULL OR ended_at >= created_at),
    FOREIGN KEY (tenant_id, student_id) REFERENCES students (tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, session_id, term_id)
        REFERENCES academic_terms (tenant_id, session_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, session_id, class_id)
        REFERENCES classes (tenant_id, session_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, class_id, class_arm_id)
        REFERENCES class_arms (tenant_id, class_id, id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX enrollments_one_active_per_student_term
    ON enrollments (tenant_id, student_id, term_id) WHERE status = 'ACTIVE';

CREATE INDEX enrollments_student_history ON enrollments (tenant_id, student_id, created_at DESC);

-- Staff profiles describe an already assigned membership. They grant no authority.
CREATE TABLE staff_profiles (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id UUID NOT NULL,
    membership_role VARCHAR(40) NOT NULL
        CHECK (membership_role IN ('SCHOOL_OWNER', 'SCHOOL_ADMIN', 'PRINCIPAL', 'TEACHER', 'BURSAR')),
    display_name VARCHAR(150) NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 150),
    staff_code CITEXT NOT NULL CHECK (length(trim(staff_code::text)) BETWEEN 1 AND 40),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, id),
    CONSTRAINT staff_profiles_tenant_user_unique UNIQUE (tenant_id, user_id),
    CONSTRAINT staff_profiles_tenant_staff_code_unique UNIQUE (tenant_id, staff_code),
    FOREIGN KEY (tenant_id, user_id, membership_role)
        REFERENCES memberships (tenant_id, user_id, role) ON DELETE RESTRICT
);

INSERT INTO schema_migrations (version) VALUES ('000009_people_enrollment')
ON CONFLICT (version) DO NOTHING;
