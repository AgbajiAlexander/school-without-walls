package curriculum

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

var errSelectionConflict = errors.New("curriculum selection changed or session is archived")

type selectionInput struct {
	Stage              string    `json:"stage"`
	CurriculumRole     string    `json:"curriculum_role"`
	FrameworkVersionID uuid.UUID `json:"framework_version_id"`
	ExpectedRevision   *int      `json:"expected_revision"`
}

type schoolSelection struct {
	ID                 uuid.UUID `json:"id"`
	Stage              string    `json:"stage"`
	CurriculumRole     string    `json:"curriculum_role"`
	Revision           int       `json:"revision"`
	FrameworkVersionID uuid.UUID `json:"framework_version_id"`
	FrameworkTitle     string    `json:"framework_title"`
	FrameworkEdition   string    `json:"framework_edition"`
	FrameworkStatus    string    `json:"framework_status"`
	SelectedAt         time.Time `json:"selected_at"`
}

type selectionScope struct {
	tenant, session, actor uuid.UUID
	status                 string
}

func (h *Handler) ListSchoolSelections(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		selectionError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	scope, err := findSelectionScope(r, tx)
	if err != nil {
		selectionError(w, r, err)
		return
	}
	rows, err := tx.Query(ctx, `
		SELECT s.id, s.stage, s.curriculum_role, s.revision, s.framework_version_id, s.selected_at,
			f.title AS framework_title, f.edition AS framework_edition, f.status AS framework_status
		FROM school_current_curriculum_selections s JOIN curriculum_framework_versions f ON f.id = s.framework_version_id
		WHERE s.tenant_id=$1 AND s.session_id=$2 ORDER BY s.stage,s.curriculum_role
	`, scope.tenant, scope.session)
	if err != nil {
		selectionError(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[schoolSelection])
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		selectionError(w, r, err)
		return
	}
	if items == nil {
		items = []schoolSelection{}
	}
	response.JSON(w, r, http.StatusOK, struct {
		Items         []schoolSelection `json:"items"`
		SessionStatus string            `json:"session_status"`
	}{Items: items, SessionStatus: scope.status})
}

func (h *Handler) CreateSchoolSelection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input selectionInput
	if !decodeAlignmentRequest(w, r, &input) || !validSelection(input) {
		invalidQuery(w, r)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		selectionError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	scope, err := findSelectionScope(r, tx)
	if err != nil {
		selectionError(w, r, err)
		return
	}
	if err := lockSelectionScope(ctx, tx, scope); err != nil {
		selectionError(w, r, err)
		return
	}
	var revision int
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(max(revision), 0) FROM school_curriculum_selections
		WHERE tenant_id=$1 AND session_id=$2 AND stage=$3 AND curriculum_role=$4
	`, scope.tenant, scope.session, input.Stage, input.CurriculumRole).Scan(&revision)
	if err != nil {
		selectionError(w, r, err)
		return
	}
	if revision != *input.ExpectedRevision {
		selectionError(w, r, errSelectionConflict)
		return
	}

	id := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO school_curriculum_selections
			(id,tenant_id,session_id,stage,curriculum_role,revision,framework_version_id,selected_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, id, scope.tenant, scope.session, input.Stage, input.CurriculumRole, revision+1, input.FrameworkVersionID, scope.actor)
	if err != nil {
		selectionError(w, r, err)
		return
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id)
		VALUES ($1,$2,$3,'curriculum.selection.create','curriculum_selection',$4,$5)
	`, uuid.New(), scope.tenant, scope.actor, id.String(), middleware.GetRequestID(ctx))
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		selectionError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, struct {
		ID       uuid.UUID `json:"id"`
		Revision int       `json:"revision"`
	}{ID: id, Revision: revision + 1})
}

func validSelection(input selectionInput) bool {
	switch input.Stage {
	case "EARLY_YEARS", "PRIMARY", "JUNIOR_SECONDARY", "SENIOR_SECONDARY":
	default:
		return false
	}
	return (input.CurriculumRole == "PRIMARY" || input.CurriculumRole == "ENRICHMENT") &&
		input.FrameworkVersionID != uuid.Nil && input.ExpectedRevision != nil && *input.ExpectedRevision >= 0
}

const managerRoles = `role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL')`

func findSelectionScope(r *http.Request, tx pgx.Tx) (selectionScope, error) {
	user := auth.GetUser(r.Context())
	session, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if user == nil || err != nil || session == uuid.Nil {
		return selectionScope{}, pgx.ErrNoRows
	}
	scope := selectionScope{actor: user.ID, session: session}
	// Deny an unauthorized request before it acquires school or session locks.
	err = tx.QueryRow(r.Context(), `
		SELECT t.id,s.status FROM tenants t JOIN academic_sessions s ON s.tenant_id=t.id
		JOIN users u ON u.id=$3 AND u.status='ACTIVE'
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND s.id=$2 AND EXISTS (
			SELECT 1 FROM memberships WHERE tenant_id=t.id AND user_id=u.id AND status='ACTIVE' AND `+managerRoles+`
		)
	`, chi.URLParam(r, "slug"), session, user.ID).Scan(&scope.tenant, &scope.status)
	return scope, err
}

func lockSelectionScope(ctx context.Context, tx pgx.Tx, scope selectionScope) error {
	// Match school lifecycle and membership writers: tenant, user, membership,
	// then academic session. The session lock serializes revision decisions.
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM tenants WHERE id=$1 AND status='ACTIVE' FOR SHARE`, scope.tenant).Scan(&id); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 AND status='ACTIVE' FOR SHARE`, scope.actor).Scan(&id); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `
		SELECT id FROM memberships WHERE tenant_id=$1 AND user_id=$2 AND status='ACTIVE' AND `+managerRoles+`
		ORDER BY id LIMIT 1 FOR SHARE
	`, scope.tenant, scope.actor).Scan(&id); err != nil {
		return err
	}
	var status string
	if err := tx.QueryRow(ctx, `
		SELECT status FROM academic_sessions WHERE tenant_id=$1 AND id=$2 FOR NO KEY UPDATE
	`, scope.tenant, scope.session).Scan(&status); err != nil {
		return err
	}
	if status == "ARCHIVED" {
		return errSelectionConflict
	}
	return nil
}

func selectionError(w http.ResponseWriter, r *http.Request, err error) {
	var dbErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		response.ErrorJSON(w, r, http.StatusNotFound, response.ErrCodeNotFound, "School session unavailable", nil)
	case errors.Is(err, errSelectionConflict):
		response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "Curriculum selection has changed or the session is archived", nil)
	case errors.As(err, &dbErr) && dbErr.Code == "23514":
		response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Edition is unavailable or does not match the stage and role", nil)
	default:
		slog.Error("school curriculum selection failed", "request_id", middleware.GetRequestID(r.Context()))
		response.ErrorJSON(w, r, http.StatusInternalServerError, response.ErrCodeInternalError, "Unable to load or save curriculum selection", nil)
	}
}
