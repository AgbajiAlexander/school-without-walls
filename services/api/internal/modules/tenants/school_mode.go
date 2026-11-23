package tenants

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

type SchoolMode struct {
	Mode        string     `json:"mode"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	Reason      *string    `json:"reason,omitempty"`
	Help        *string    `json:"help,omitempty"`
	ScheduleURL *string    `json:"schedule_url,omitempty"`
}

type SchoolModeHandler struct{ pool *pgxpool.Pool }

func NewSchoolModeHandler(pool *pgxpool.Pool) *SchoolModeHandler {
	return &SchoolModeHandler{pool: pool}
}

func (h *SchoolModeHandler) Update(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := tenantActor(w, r)
	if !ok {
		return
	}
	var input SchoolMode
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || !errors.Is(decoder.Decode(new(any)), io.EOF) || !validSchoolMode(&input) {
		invalidMode(w, r)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		modeFailure(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var tenant uuid.UUID
	var previous string
	slug := chi.URLParam(r, "slug")
	const adminMembership = ` FROM memberships m JOIN users u ON u.id=m.user_id AND u.status='ACTIVE'
		WHERE m.tenant_id=t.id AND m.user_id=$2 AND m.status='ACTIVE'
		AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL')`
	err = tx.QueryRow(ctx, `SELECT t.id,t.operating_mode FROM tenants t
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND EXISTS (SELECT 1`+adminMembership+`)
		FOR UPDATE OF t`, slug, actor.ID).Scan(&tenant, &previous)
	if err != nil {
		modeFailure(w, r, err)
		return
	}
	var member uuid.UUID
	err = tx.QueryRow(ctx, `SELECT m.id FROM tenants t JOIN LATERAL (SELECT m.id`+adminMembership+`
		ORDER BY m.id LIMIT 1 FOR SHARE OF u,m) m ON true WHERE t.id=$1`, tenant, actor.ID).Scan(&member)
	if err != nil {
		modeFailure(w, r, err)
		return
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		modeFailure(w, r, err)
		return
	}
	if input.EndsAt != nil && !input.EndsAt.After(now) {
		invalidMode(w, r)
		return
	}
	_, err = tx.Exec(ctx, `UPDATE tenants SET operating_mode=$2,mode_starts_at=$3,mode_ends_at=$4,mode_reason=$5,
		mode_help=$6,mode_schedule_url=$7,updated_at=clock_timestamp() WHERE id=$1`,
		tenant, input.Mode, input.StartsAt, input.EndsAt, input.Reason, input.Help, input.ScheduleURL)
	if err == nil {
		err = writeAuditEvent(ctx, tx, &auth.AuditEvent{TenantID: &tenant, ActorID: &actor.ID, Action: "tenant.operating_mode.update",
			SubjectType: "tenant", SubjectID: tenant.String(), RequestID: middleware.GetRequestID(ctx),
			MetadataRedacted: map[string]any{"previous_mode": previous, "mode": input.Mode, "starts_at": input.StartsAt, "ends_at": input.EndsAt}})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		modeFailure(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, input)
}

func validSchoolMode(input *SchoolMode) bool {
	if input.Mode == "ON_SITE" {
		return input.StartsAt == nil && input.EndsAt == nil && input.Reason == nil && input.Help == nil && input.ScheduleURL == nil
	}
	if (input.Mode != "HYBRID" && input.Mode != "REMOTE") || input.StartsAt == nil || input.EndsAt == nil ||
		!input.EndsAt.After(*input.StartsAt) || input.Reason == nil {
		return false
	}
	for _, field := range []struct {
		value *string
		max   int
	}{{input.Reason, 500}, {input.Help, 1000}, {input.ScheduleURL, 2048}} {
		if field.value == nil {
			continue
		}
		*field.value = strings.TrimSpace(*field.value)
		if *field.value == "" || utf8.RuneCountInString(*field.value) > field.max || strings.ContainsRune(*field.value, 0) {
			return false
		}
	}
	if input.ScheduleURL != nil {
		u, err := url.Parse(*input.ScheduleURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || strings.ContainsAny(*input.ScheduleURL, "\\\r\n\t ") {
			return false
		}
	}
	return true
}

func invalidMode(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Check the school mode, schedule and reason", nil)
}

func modeFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		response.ErrorJSON(w, r, http.StatusNotFound, response.ErrCodeNotFound, "School mode is unavailable", nil)
		return
	}
	response.ErrorJSON(w, r, http.StatusInternalServerError, response.ErrCodeInternalError, "Could not save school mode", nil)
}
