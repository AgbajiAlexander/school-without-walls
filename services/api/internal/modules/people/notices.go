package people

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

type noticeInput struct {
	Title    string     `json:"title"`
	Body     string     `json:"body"`
	Audience string     `json:"audience"`
	Role     *string    `json:"role"`
	Stage    *string    `json:"stage"`
	ClassID  *uuid.UUID `json:"class_id"`
}

type schoolNotice struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
}

func (h *Handler) ListNotices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor := auth.GetUser(r.Context())
	if actor == nil {
		writeError(w, r, ErrNotFound)
		return
	}
	rows, err := h.repo.pool.Query(r.Context(), `
		SELECT n.id,n.title,n.body,n.published_at
		FROM school_notices n JOIN tenants t ON t.id=n.tenant_id JOIN users u ON u.id=$2 AND u.status='ACTIVE'
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND EXISTS (
			SELECT 1 FROM memberships m WHERE m.tenant_id=t.id AND m.user_id=u.id AND m.status='ACTIVE'
				AND (n.audience='SCHOOL' OR (n.audience='ROLE' AND n.target_role=m.role)
				OR (n.audience IN ('STAGE','CLASS') AND m.role IN ('STUDENT','PARENT') AND EXISTS (
					SELECT 1 FROM students s JOIN enrollments e ON e.tenant_id=s.tenant_id AND e.student_id=s.id AND e.status='ACTIVE'
					JOIN classes c ON c.tenant_id=e.tenant_id AND c.id=e.class_id AND c.status='ACTIVE'
					JOIN academic_sessions sess ON sess.tenant_id=e.tenant_id AND sess.id=e.session_id AND sess.status='ACTIVE'
					JOIN academic_terms term ON term.tenant_id=e.tenant_id AND term.id=e.term_id
					JOIN grade_levels grade ON grade.tenant_id=c.tenant_id AND grade.id=c.grade_level_id AND grade.status='ACTIVE'
					LEFT JOIN class_arms arm ON arm.tenant_id=e.tenant_id AND arm.id=e.class_arm_id
					WHERE s.tenant_id=t.id AND s.status='ACTIVE'
					  AND (statement_timestamp() AT TIME ZONE t.timezone)::date BETWEEN term.starts_on AND term.ends_on
					  AND (e.class_arm_id IS NULL OR arm.status='ACTIVE')
					  AND ((n.audience='CLASS' AND c.id=n.target_class_id) OR (n.audience='STAGE' AND grade.stage=n.target_stage))
					  AND ((m.role='STUDENT' AND s.user_id=u.id) OR (m.role='PARENT' AND EXISTS (
					    SELECT 1 FROM guardians guardian JOIN student_guardians link ON link.tenant_id=guardian.tenant_id
							AND link.guardian_id=guardian.id AND link.active
					    WHERE guardian.tenant_id=t.id AND guardian.user_id=u.id AND link.student_id=s.id
					  )))
				)))
		) ORDER BY n.published_at DESC,n.id DESC LIMIT 50`, chi.URLParam(r, "slug"), actor.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[schoolNotice])
	if err != nil {
		writeError(w, r, err)
		return
	}
	if items == nil {
		items = []schoolNotice{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) NoticeClasses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor := auth.GetUser(r.Context())
	if actor == nil {
		writeError(w, r, ErrNotFound)
		return
	}
	type class struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}
	rows, err := h.repo.pool.Query(r.Context(), `SELECT c.id,c.name FROM tenants t JOIN users u ON u.id=$2 AND u.status='ACTIVE'
		JOIN academic_sessions sess ON sess.tenant_id=t.id AND sess.status='ACTIVE'
		JOIN classes c ON c.tenant_id=t.id AND c.session_id=sess.id AND c.status='ACTIVE'
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND EXISTS (
			SELECT 1 FROM memberships m WHERE m.tenant_id=t.id AND m.user_id=u.id AND m.status='ACTIVE'
			AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL')) ORDER BY c.name,c.id`, chi.URLParam(r, "slug"), actor.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[class])
	if err != nil {
		writeError(w, r, err)
		return
	}
	if items == nil {
		items = []class{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) PublishNotice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor := auth.GetUser(r.Context())
	if actor == nil {
		writeError(w, r, ErrNotFound)
		return
	}
	var input noticeInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || !errors.Is(decoder.Decode(new(any)), io.EOF) || !validNotice(&input) {
		response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Check the notice and its audience", nil)
		return
	}
	ctx := r.Context()
	tx, err := h.repo.pool.Begin(ctx)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	tenant, err := lockManager(ctx, tx, chi.URLParam(r, "slug"), actor.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if input.ClassID != nil {
		var id uuid.UUID
		err = tx.QueryRow(ctx, `SELECT c.id FROM classes c JOIN academic_sessions s ON s.tenant_id=c.tenant_id
			AND s.id=c.session_id AND s.status='ACTIVE' WHERE c.tenant_id=$1 AND c.id=$2 AND c.status='ACTIVE'
			FOR SHARE OF c,s`, tenant, *input.ClassID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Check the notice and its audience", nil)
			return
		}
		if err != nil {
			writeError(w, r, err)
			return
		}
	}
	notice := schoolNotice{ID: uuid.New(), Title: input.Title, Body: input.Body}
	err = tx.QueryRow(ctx, `INSERT INTO school_notices (id,tenant_id,published_by,title,body,audience,target_role,target_stage,target_class_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING published_at`,
		notice.ID, tenant, actor.ID, input.Title, input.Body, input.Audience, input.Role, input.Stage, input.ClassID).Scan(&notice.PublishedAt)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id,metadata_redacted)
			VALUES ($1,$2,$3,'notice.publish','school_notice',$4,$5,jsonb_build_object('audience',$6::text))`,
			uuid.New(), tenant, actor.ID, notice.ID.String(), middleware.GetRequestID(ctx), input.Audience)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, notice)
}

func validNotice(input *noticeInput) bool {
	input.Title = strings.TrimSpace(input.Title)
	input.Body = strings.TrimSpace(input.Body)
	if input.Title == "" || input.Body == "" || utf8.RuneCountInString(input.Title) > 160 || utf8.RuneCountInString(input.Body) > 4000 ||
		strings.ContainsRune(input.Title, 0) || strings.ContainsRune(input.Body, 0) {
		return false
	}
	switch input.Audience {
	case "SCHOOL":
		return input.Role == nil && input.Stage == nil && input.ClassID == nil
	case "ROLE":
		return input.Role != nil && slices.Contains([]string{"SCHOOL_OWNER", "SCHOOL_ADMIN", "PRINCIPAL", "TEACHER", "BURSAR", "STUDENT", "PARENT"}, *input.Role) &&
			input.Stage == nil && input.ClassID == nil
	case "STAGE":
		return input.Stage != nil && slices.Contains([]string{"EARLY_YEARS", "PRIMARY", "JUNIOR_SECONDARY", "SENIOR_SECONDARY"}, *input.Stage) &&
			input.Role == nil && input.ClassID == nil
	case "CLASS":
		return input.ClassID != nil && *input.ClassID != uuid.Nil && input.Role == nil && input.Stage == nil
	default:
		return false
	}
}
