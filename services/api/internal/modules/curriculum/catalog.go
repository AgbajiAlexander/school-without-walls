package curriculum

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

const pageSize = 50

var errMembershipRequired = errors.New("active school membership required")

type Handler struct{ pool *pgxpool.Pool }

func NewHandler(pool *pgxpool.Pool) *Handler { return &Handler{pool: pool} }

type framework struct {
	ID            uuid.UUID `json:"id"`
	Jurisdiction  string    `json:"jurisdiction"`
	Publisher     string    `json:"publisher"`
	Title         string    `json:"title"`
	Edition       string    `json:"edition"`
	FrameworkKind string    `json:"framework_kind"`
	EffectiveFrom string    `json:"effective_from"`
	EffectiveTo   *string   `json:"effective_to"`
	SourceURL     string    `json:"source_url"`
	LicenseNote   string    `json:"license_note"`
	Status        string    `json:"status"`
}

type objective struct {
	ID                 uuid.UUID `json:"id"`
	FrameworkVersionID uuid.UUID `json:"framework_version_id"`
	Stage              string    `json:"stage"`
	GradeBand          string    `json:"grade_band"`
	Subject            string    `json:"subject"`
	Strand             string    `json:"strand"`
	Code               string    `json:"code"`
	ObjectiveText      string    `json:"objective_text"`
}

func (h *Handler) ListFrameworks(w http.ResponseWriter, r *http.Request) {
	browse(h, w, r, pgx.RowToStructByName[framework], func(item framework) uuid.UUID { return item.ID },
		`SELECT id, jurisdiction, publisher, title, edition, framework_kind,
			to_char(effective_from, 'YYYY-MM-DD') AS effective_from,
			to_char(effective_to, 'YYYY-MM-DD') AS effective_to, source_url, license_note, status
		  FROM curriculum_framework_versions
		  WHERE status = 'PUBLISHED' AND ($1::uuid IS NULL OR id > $1)
		  ORDER BY id LIMIT $2`, nil)
}

func (h *Handler) ListObjectives(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	id, err := uuid.Parse(q.Get("framework_version_id"))
	if err != nil || id == uuid.Nil {
		invalidQuery(w, r)
		return
	}
	stage := q.Get("stage")
	switch stage {
	case "", "EARLY_YEARS", "PRIMARY", "JUNIOR_SECONDARY", "SENIOR_SECONDARY":
	default:
		invalidQuery(w, r)
		return
	}
	browse(h, w, r, pgx.RowToStructByName[objective], func(item objective) uuid.UUID { return item.ID },
		`SELECT o.id, o.framework_version_id, o.stage, o.grade_band, o.subject, o.strand, o.code, o.objective_text
		  FROM curriculum_objectives o JOIN curriculum_framework_versions f ON f.id = o.framework_version_id
		  WHERE f.status = 'PUBLISHED' AND o.framework_version_id = $3
		    AND ($4::text = '' OR o.stage = $4) AND ($1::uuid IS NULL OR o.id > $1)
		  ORDER BY o.id LIMIT $2`, []any{id, stage})
}

func browse[T any](h *Handler, w http.ResponseWriter, r *http.Request,
	scan pgx.RowToFunc[T], identify func(T) uuid.UUID, query string, filters []any,
) {
	w.Header().Set("Cache-Control", "no-store")
	user := auth.GetUser(r.Context())
	if user == nil || user.ID == uuid.Nil {
		response.ErrorJSON(w, r, http.StatusUnauthorized, response.ErrCodeUnauthenticated, "Authentication required", nil)
		return
	}
	var after *uuid.UUID
	if raw := r.URL.Query().Get("after"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			invalidQuery(w, r)
			return
		}
		after = &id
	}
	items, err := readCatalog(r.Context(), h.pool, user.ID, scan, query, append([]any{after, pageSize + 1}, filters...))
	if errors.Is(err, errMembershipRequired) {
		response.ErrorJSON(w, r, http.StatusForbidden, response.ErrCodeForbidden, "Active school membership required", nil)
		return
	}
	if err != nil {
		slog.Error("curriculum registry read failed", "request_id", middleware.GetRequestID(r.Context()))
		response.ErrorJSON(w, r, http.StatusInternalServerError, response.ErrCodeInternalError, "Curriculum registry is unavailable", nil)
		return
	}
	var next *uuid.UUID
	if len(items) > pageSize {
		items = items[:pageSize]
		id := identify(items[pageSize-1])
		next = &id
	}
	response.JSON(w, r, http.StatusOK, struct {
		Items []T        `json:"items"`
		Next  *uuid.UUID `json:"next,omitempty"`
	}{Items: items, Next: next})
}

func readCatalog[T any](ctx context.Context, pool *pgxpool.Pool, actor uuid.UUID,
	scan pgx.RowToFunc[T], query string, args []any,
) ([]T, error) {
	// Authorization and the catalog read share a snapshot. A committed
	// suspension is visible on the next request.
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var allowed bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM memberships m
		JOIN tenants t ON t.id = m.tenant_id
		JOIN users u ON u.id = m.user_id
		WHERE m.user_id = $1 AND m.status = 'ACTIVE' AND t.status = 'ACTIVE' AND u.status = 'ACTIVE'
	)`, actor).Scan(&allowed)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errMembershipRequired
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	items, err := pgx.CollectRows(rows, scan)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if items == nil {
		items = []T{}
	}
	return items, nil
}

func invalidQuery(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid curriculum query", nil)
}
