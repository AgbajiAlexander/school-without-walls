package curriculum

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

var errReviewerRequired = errors.New("global curriculum administrator required")

type alignmentInput struct {
	PrimaryObjectiveID uuid.UUID `json:"primary_objective_id"`
	RelatedObjectiveID uuid.UUID `json:"related_objective_id"`
	Relation           string    `json:"relation"`
}

type verifiedAlignment struct {
	ID                 uuid.UUID `json:"id"`
	PrimaryObjectiveID uuid.UUID `json:"primary_objective_id"`
	RelatedObjectiveID uuid.UUID `json:"related_objective_id"`
	Relation           string    `json:"relation"`
	ReviewedAt         time.Time `json:"reviewed_at"`
	RelatedCode        string    `json:"related_code"`
	RelatedText        string    `json:"related_text"`
	FrameworkTitle     string    `json:"framework_title"`
	FrameworkEdition   string    `json:"framework_edition"`
	FrameworkPublisher string    `json:"framework_publisher"`
	FrameworkSourceURL string    `json:"framework_source_url"`
	FrameworkLicense   string    `json:"framework_license"`
}

func (h *Handler) ListVerifiedAlignments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, err := uuid.Parse(r.URL.Query().Get("primary_objective_id"))
	if err != nil || id == uuid.Nil {
		invalidQuery(w, r)
		return
	}
	browse(h, w, r, pgx.RowToStructByName[verifiedAlignment], func(a verifiedAlignment) uuid.UUID { return a.ID },
		`SELECT a.id, a.primary_objective_id, a.related_objective_id, a.relation, a.reviewed_at,
			o.code AS related_code, o.objective_text AS related_text, f.title AS framework_title,
			f.edition AS framework_edition, f.publisher AS framework_publisher,
			f.source_url AS framework_source_url, f.license_note AS framework_license
		FROM curriculum_verified_alignments a
		JOIN curriculum_objectives o ON o.id = a.related_objective_id
		JOIN curriculum_framework_versions f ON f.id = o.framework_version_id
		WHERE a.primary_objective_id = $3 AND ($1::uuid IS NULL OR a.id > $1)
		ORDER BY a.id LIMIT $2`, []any{id})
}

func (h *Handler) CreateAlignment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input alignmentInput
	if !decodeAlignmentRequest(w, r, &input) ||
		input.PrimaryObjectiveID == uuid.Nil || input.RelatedObjectiveID == uuid.Nil ||
		input.PrimaryObjectiveID == input.RelatedObjectiveID {
		invalidAlignment(w, r)
		return
	}
	switch input.Relation {
	case "EQUIVALENT", "COMPLEMENTARY", "EXTENDS", "DIFFERENT":
	default:
		invalidAlignment(w, r)
		return
	}
	id := uuid.New()
	h.writeAlignment(w, r, id, false, func(ctx context.Context, tx pgx.Tx, _ uuid.UUID) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO curriculum_objective_alignments (id, primary_objective_id, related_objective_id, relation)
			VALUES ($1, $2, $3, $4)
		`, id, input.PrimaryObjectiveID, input.RelatedObjectiveID, input.Relation)
		return err
	})
}

func (h *Handler) ReviewAlignment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, err := uuid.Parse(chi.URLParam(r, "alignmentID"))
	if err != nil || id == uuid.Nil || !decodeAlignmentRequest(w, r, &struct{}{}) {
		invalidAlignment(w, r)
		return
	}
	h.writeAlignment(w, r, id, true, func(ctx context.Context, tx pgx.Tx, actor uuid.UUID) error {
		var reviewed uuid.UUID
		return tx.QueryRow(ctx, `
			UPDATE curriculum_objective_alignments SET review_status = 'HUMAN_REVIEWED', reviewer_id = $2
			WHERE id = $1 AND review_status = 'DRAFT' RETURNING id
		`, id, actor).Scan(&reviewed)
	})
}

func (h *Handler) writeAlignment(w http.ResponseWriter, r *http.Request, id uuid.UUID, review bool,
	write func(context.Context, pgx.Tx, uuid.UUID) error,
) {
	actor := auth.GetUser(r.Context())
	if actor == nil || actor.ID == uuid.Nil {
		response.ErrorJSON(w, r, http.StatusUnauthorized, response.ErrCodeUnauthenticated, "Authentication required", nil)
		return
	}
	action := "curriculum.alignment.create"
	status, httpStatus := "DRAFT", http.StatusCreated
	if review {
		action = "curriculum.alignment.review"
		status, httpStatus = "HUMAN_REVIEWED", http.StatusOK
	}
	ctx := r.Context()
	requestID := middleware.GetRequestID(ctx)
	err := h.mutateAlignment(ctx, actor.ID, id, action, requestID, write)
	if err != nil {
		var dbErr *pgconn.PgError
		switch {
		case errors.Is(err, errReviewerRequired):
			response.ErrorJSON(w, r, http.StatusForbidden, response.ErrCodeForbidden, "Global curriculum administrator access required", nil)
		case errors.Is(err, pgx.ErrNoRows):
			response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "No draft alignment is available for review", nil)
		case errors.As(err, &dbErr) && (dbErr.Code == "23505" || (review && dbErr.Code == "23514")):
			response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "The alignment conflicts with the current catalog state", nil)
		case dbErr != nil && (dbErr.Code == "23514" || dbErr.Code == "23503"):
			invalidAlignment(w, r)
		default:
			slog.Error("curriculum alignment write failed", "request_id", requestID)
			response.ErrorJSON(w, r, http.StatusInternalServerError, response.ErrCodeInternalError, "Unable to save curriculum alignment", nil)
		}
		return
	}
	response.JSON(w, r, httpStatus, struct {
		ID           uuid.UUID `json:"id"`
		ReviewStatus string    `json:"review_status"`
	}{ID: id, ReviewStatus: status})
}

func (h *Handler) mutateAlignment(ctx context.Context, actor, id uuid.UUID, action, requestID string,
	write func(context.Context, pgx.Tx, uuid.UUID) error,
) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// A suspension or grant removal committed before this lock denies the write.
	var authorized uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT u.id FROM users u JOIN platform_admins p ON p.user_id = u.id
		WHERE u.id = $1 AND u.status = 'ACTIVE' FOR SHARE OF u, p
	`, actor).Scan(&authorized)
	if errors.Is(err, pgx.ErrNoRows) {
		return errReviewerRequired
	}
	if err != nil {
		return err
	}
	if err := write(ctx, tx, actor); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (id, actor_id, action, subject_type, subject_id, request_id)
		VALUES ($1, $2, $3, 'curriculum_alignment', $4, $5)
	`, uuid.New(), actor, action, id.String(), requestID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func decodeAlignmentRequest(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst) == nil && errors.Is(decoder.Decode(new(any)), io.EOF)
}

func invalidAlignment(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid curriculum alignment", nil)
}
