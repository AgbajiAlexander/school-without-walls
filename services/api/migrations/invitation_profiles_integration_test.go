package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/modules/auth"
)

func testProfileInvitationRedemption(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	school, other, student, guardian := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO tenants (id, slug, name, status) VALUES
        ($1, 'invite-profile-a', 'Fictional A', 'ACTIVE'),
        ($2, 'invite-profile-b', 'Fictional B', 'ACTIVE')`, school, other)
	exec(`INSERT INTO students (id, tenant_id, admission_no, display_name, stage)
        VALUES ($1, $2, 'INV-FIXTURE', 'Fictional Student', 'PRIMARY')`, student, school)
	exec(`INSERT INTO guardians (id, tenant_id, display_name, contact_email)
        VALUES ($1, $2, 'Fictional Guardian', 'guardian-fixture@example.test')`, guardian, school)

	const insertInvitation = `INSERT INTO invitations
        (tenant_id, invited_email, target_role, student_id, guardian_id, token_hash, status, expires_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,clock_timestamp() + interval '1 hour') RETURNING id`
	_, err := pool.Exec(ctx, insertInvitation, other, "foreign-fixture@example.test", "STUDENT", student, nil, auth.HashToken(uuid.NewString()), "PENDING")
	var dbErr *pgconn.PgError
	if !errors.As(err, &dbErr) || dbErr.ConstraintName != "invitations_student_profile_fk" {
		t.Fatal("a student invitation must reference a profile in the same school")
	}

	service := auth.NewService(auth.NewPostgresRepository(pool))
	for _, tc := range []struct {
		role, recipient       string
		studentID, guardianID *uuid.UUID
	}{
		{"STUDENT", "student-fixture@example.test", &student, nil},
		{"PARENT", "guardian-fixture@example.test", nil, &guardian},
	} {
		t.Run(tc.role, func(t *testing.T) {
			token := uuid.NewString()
			var id uuid.UUID
			if err := pool.QueryRow(ctx, insertInvitation, school, tc.recipient, tc.role,
				tc.studentID, tc.guardianID, auth.HashToken(token), "AWAITING_DELIVERY").Scan(&id); err != nil {
				t.Fatal(err)
			}
			accept := func() (*auth.User, error) {
				user, _, _, err := service.AcceptInvitation(ctx, token, "Fictional Invitee",
					"Sww-fixture-passphrase37!", "127.0.0.1", "integration", "profile-redemption")
				return user, err
			}
			assertNoAccess := func() {
				t.Helper()
				var count int
				if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE email = $1`, tc.recipient).Scan(&count); err != nil || count != 0 {
					t.Fatal("rejected invitation left an account")
				}
			}
			if _, err := accept(); !errors.Is(err, auth.ErrInvitationNotFound) {
				t.Fatal("unconfirmed invitation must be rejected")
			}
			assertNoAccess()
			exec(`UPDATE invitations SET status = 'PENDING' WHERE id = $1`, id)
			if tc.role == "PARENT" {
				exec(`UPDATE guardians SET contact_email = 'changed-fixture@example.test' WHERE id = $1`, guardian)
				if _, err := accept(); !errors.Is(err, auth.ErrInvitationNotFound) {
					t.Fatal("invitation to a superseded guardian address must be rejected")
				}
				assertNoAccess()
				exec(`UPDATE guardians SET contact_email = $2 WHERE id = $1`, guardian, tc.recipient)
			}
			user, err := accept()
			if err != nil {
				t.Fatal(err)
			}
			query := `SELECT user_id FROM students WHERE id = $1`
			profile := student
			if tc.role == "PARENT" {
				query = `SELECT user_id FROM guardians WHERE id = $1`
				profile = guardian
			}
			var bound uuid.UUID
			if err := pool.QueryRow(ctx, query, profile).Scan(&bound); err != nil || bound != user.ID || user.Email != tc.recipient {
				t.Fatal("successful redemption did not bind the intended profile and account")
			}
			if _, err := accept(); !errors.Is(err, auth.ErrInvitationNotFound) {
				t.Fatal("consumed invitation must be rejected on replay")
			}
		})
	}
}
