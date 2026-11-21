-- Profile invitations must name exactly one profile in the same school.
ALTER TABLE invitations
    ADD COLUMN student_id UUID,
    ADD COLUMN guardian_id UUID,
    ADD CONSTRAINT invitations_student_profile_fk FOREIGN KEY (tenant_id, student_id)
        REFERENCES students (tenant_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT invitations_guardian_profile_fk FOREIGN KEY (tenant_id, guardian_id)
        REFERENCES guardians (tenant_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT invitations_profile_target_check CHECK (
        CASE target_role
            WHEN 'STUDENT' THEN tenant_id IS NOT NULL AND student_id IS NOT NULL AND guardian_id IS NULL
            WHEN 'PARENT' THEN tenant_id IS NOT NULL AND guardian_id IS NOT NULL AND student_id IS NULL
            ELSE student_id IS NULL AND guardian_id IS NULL
        END
    );

CREATE INDEX invitations_student_profile_idx ON invitations (tenant_id, student_id) WHERE student_id IS NOT NULL;
CREATE INDEX invitations_guardian_profile_idx ON invitations (tenant_id, guardian_id) WHERE guardian_id IS NOT NULL;
