package learning

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/media"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

type Handler struct {
	pool         *pgxpool.Pool
	mediaStore   *media.Store
	mediaScanner media.Scanner
	scanSlots    chan struct{}
}

func NewHandler(pool *pgxpool.Pool) *Handler { return &Handler{pool: pool} }

type publishedLesson struct {
	ProgressState string             `json:"progress_state"`
	HasAttachment bool               `json:"has_attachment,omitempty"`
	ID            uuid.UUID          `json:"id"`
	Title         string             `json:"title"`
	Subject       string             `json:"subject"`
	ObjectiveCode string             `json:"objective_code"`
	PublishedAt   time.Time          `json:"published_at"`
	Content       *string            `json:"content,omitempty"`
	Enrichment    []lessonEnrichment `json:"enrichment,omitempty"`
}

type lessonEnrichment struct {
	Code      string `json:"code"`
	Text      string `json:"text"`
	Withdrawn bool   `json:"withdrawn"`
}

const lessonEnrichmentDetails = `(
			SELECT jsonb_agg(jsonb_build_object('code',r.code,'text',r.objective_text,'withdrawn',f.status='WITHDRAWN') ORDER BY r.code,a.id)
			FROM curriculum_objective_alignments a JOIN curriculum_objectives r ON r.id=a.related_objective_id
			JOIN curriculum_framework_versions f ON f.id=r.framework_version_id
			WHERE a.id=ANY(l.enrichment_alignment_ids)
		)`

const studentLessonQuery = `
	SELECT l.id,l.title,subject.name AS subject,objective.code AS objective_code,l.published_at,
		CASE WHEN $3::uuid IS NOT NULL THEN l.content END AS content,
		CASE WHEN $3::uuid IS NOT NULL THEN ` + lessonEnrichmentDetails + ` END AS enrichment,
		($5::boolean AND $3::uuid IS NOT NULL AND ` + readyLessonAttachment + `) AS has_attachment,
		COALESCE((SELECT ` + progressState + ` FROM lesson_progress progress
			WHERE progress.tenant_id=t.id AND progress.lesson_id=l.id AND progress.student_id=s.id), 'NOT_STARTED') AS progress_state` + studentLessonScope + ` ORDER BY l.id LIMIT 51`

const studentLessonScope = `
	FROM tenants t JOIN users u ON u.id=$2 AND u.status='ACTIVE'
	JOIN memberships m ON m.tenant_id=t.id AND m.user_id=u.id AND m.role='STUDENT' AND m.status='ACTIVE'
	JOIN students s ON s.tenant_id=t.id AND s.user_id=u.id AND s.status='ACTIVE'
	JOIN enrollments e ON e.tenant_id=t.id AND e.student_id=s.id AND e.status='ACTIVE'
	JOIN academic_sessions session ON session.tenant_id=t.id AND session.id=e.session_id AND session.status='ACTIVE'
	JOIN academic_terms term ON term.tenant_id=t.id AND term.session_id=e.session_id AND term.id=e.term_id
	JOIN classes c ON c.tenant_id=t.id AND c.session_id=e.session_id AND c.id=e.class_id AND c.status='ACTIVE'
	JOIN grade_levels g ON g.tenant_id=t.id AND g.id=c.grade_level_id AND g.status='ACTIVE'
	LEFT JOIN class_arms arm ON arm.tenant_id=t.id AND arm.class_id=c.id AND arm.id=e.class_arm_id
	JOIN subject_offerings o ON o.tenant_id=t.id AND o.term_id=term.id AND o.class_id=c.id
		AND (o.class_arm_id IS NULL OR o.class_arm_id=e.class_arm_id) AND o.status='ACTIVE'
	JOIN subjects subject ON subject.tenant_id=t.id AND subject.id=o.subject_id AND subject.status='ACTIVE'
	JOIN lessons l ON l.tenant_id=t.id AND l.offering_id=o.id AND l.status='PUBLISHED'
	JOIN curriculum_objectives objective ON objective.id=l.primary_objective_id
	WHERE t.slug=$1::citext AND t.status='ACTIVE'
	  AND (e.class_arm_id IS NULL OR arm.status='ACTIVE')
	  AND (statement_timestamp() AT TIME ZONE t.timezone)::date BETWEEN term.starts_on AND term.ends_on
	  AND l.published_at <= statement_timestamp()
	  AND ($3::uuid IS NULL OR l.id=$3) AND ($4::uuid IS NULL OR l.id>$4)
`

func (h *Handler) ListStudentLessons(w http.ResponseWriter, r *http.Request) {
	h.studentLessons(w, r, nil)
}

func (h *Handler) GetStudentLesson(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "lessonID"))
	if err != nil || id == uuid.Nil {
		w.Header().Set("Cache-Control", "no-store")
		lessonUnavailable(w, r)
		return
	}
	h.studentLessons(w, r, &id)
}

func (h *Handler) studentLessons(w http.ResponseWriter, r *http.Request, id *uuid.UUID) {
	w.Header().Set("Cache-Control", "no-store")
	actor := auth.GetUser(r.Context())
	if actor == nil || actor.ID == uuid.Nil {
		response.ErrorJSON(w, r, http.StatusUnauthorized, response.ErrCodeUnauthenticated, "Authentication required", nil)
		return
	}
	var after *uuid.UUID
	if raw := r.URL.Query().Get("after"); raw != "" && id == nil {
		cursor, err := uuid.Parse(raw)
		if err != nil || cursor == uuid.Nil {
			response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid lesson cursor", nil)
			return
		}
		after = &cursor
	}
	rows, err := h.pool.Query(r.Context(), studentLessonQuery, chi.URLParam(r, "slug"), actor.ID, id, after, h.mediaStore != nil)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	if id != nil {
		lesson, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[publishedLesson])
		if err != nil {
			lessonReadError(w, r, err)
			return
		}
		response.JSON(w, r, http.StatusOK, lesson)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[publishedLesson])
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	if items == nil {
		items = []publishedLesson{}
	}
	var next *uuid.UUID
	if len(items) > 50 {
		items = items[:50]
		key := items[49].ID
		next = &key
	}
	response.JSON(w, r, http.StatusOK, struct {
		Items []publishedLesson `json:"items"`
		Next  *uuid.UUID        `json:"next,omitempty"`
	}{Items: items, Next: next})
}

func lessonUnavailable(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusNotFound, response.ErrCodeNotFound, "Lesson unavailable", nil)
}

func lessonReadError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		lessonUnavailable(w, r)
		return
	}
	slog.Error("student lesson read failed", "request_id", middleware.GetRequestID(r.Context()))
	response.ErrorJSON(w, r, http.StatusInternalServerError, response.ErrCodeInternalError, "Unable to load lessons", nil)
}
