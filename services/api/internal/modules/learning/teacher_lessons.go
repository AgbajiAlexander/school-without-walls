package learning

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

type lessonDraft struct {
	OfferingID  uuid.UUID `json:"offering_id"`
	ObjectiveID uuid.UUID `json:"primary_objective_id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
}

var errLessonConflict = errors.New("lesson state changed")

// The author must retain the matching assignment for every mutation.
const teacherOfferingJoins = `
 FROM tenants t
 JOIN users u ON u.id=$2 AND u.status='ACTIVE'
 JOIN memberships m ON m.tenant_id=t.id AND m.user_id=u.id AND m.role='TEACHER' AND m.status='ACTIVE'
 JOIN teacher_assignments a ON a.tenant_id=t.id AND a.teacher_membership_id=m.id AND a.status='ACTIVE'
 JOIN subject_offerings o ON o.tenant_id=t.id AND o.id=a.offering_id AND o.status='ACTIVE'
`

const teacherOffering = teacherOfferingJoins + `
 WHERE t.slug=$1::citext AND t.status='ACTIVE' AND o.id=$3
`

func (h *Handler) CreateTeacherLesson(w http.ResponseWriter, r *http.Request) {
	h.writeTeacherLesson(w, r, "create")
}
func (h *Handler) UpdateTeacherLesson(w http.ResponseWriter, r *http.Request) {
	h.writeTeacherLesson(w, r, "edit")
}
func (h *Handler) SubmitTeacherLesson(w http.ResponseWriter, r *http.Request) {
	h.writeTeacherLesson(w, r, "submit")
}

func (h *Handler) PublishTeacherLesson(w http.ResponseWriter, r *http.Request) {
	h.writeTeacherLesson(w, r, "publish")
}
func (h *Handler) ArchiveTeacherLesson(w http.ResponseWriter, r *http.Request) {
	h.writeTeacherLesson(w, r, "archive")
}

func (h *Handler) writeTeacherLesson(w http.ResponseWriter, r *http.Request, action string) {
	w.Header().Set("Cache-Control", "no-store")
	actor := auth.GetUser(r.Context())
	if actor == nil {
		response.ErrorJSON(w, r, http.StatusUnauthorized, response.ErrCodeUnauthenticated, "Authentication required", nil)
		return
	}
	var draft lessonDraft
	var id uuid.UUID
	if action == "create" {
		if !decodeLesson(w, r, &draft) || draft.OfferingID == uuid.Nil || draft.ObjectiveID == uuid.Nil || !validLessonText(&draft.Title, &draft.Content) {
			invalidLesson(w, r)
			return
		}
		id = uuid.New()
	} else {
		var err error
		id, err = uuid.Parse(chi.URLParam(r, "lessonID"))
		if err != nil || id == uuid.Nil {
			lessonUnavailable(w, r)
			return
		}
		if action == "edit" {
			var text struct {
				Title   string `json:"title"`
				Content string `json:"content"`
			}
			if !decodeLesson(w, r, &text) || !validLessonText(&text.Title, &text.Content) {
				invalidLesson(w, r)
				return
			}
			draft.Title, draft.Content = text.Title, text.Content
		}
	}
	ctx := r.Context()
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	slug := chi.URLParam(r, "slug")

	if action != "create" {
		// This lookup can reveal nothing until the matching tenant and author are verified.
		err = tx.QueryRow(ctx, `SELECT l.offering_id FROM lessons l
   JOIN tenants t ON t.id=l.tenant_id JOIN memberships m ON m.tenant_id=l.tenant_id AND m.id=l.author_membership_id
   WHERE t.slug=$1::citext AND l.id=$2 AND m.user_id=$3`, slug, id, actor.ID).Scan(&draft.OfferingID)
		if err != nil {
			teacherLessonError(w, r, err)
			return
		}
	}
	var tenant, membership uuid.UUID
	err = tx.QueryRow(ctx, `SELECT t.id,m.id`+teacherOffering+` FOR SHARE OF t,u,m,a,o`, slug, actor.ID, draft.OfferingID).Scan(&tenant, &membership)
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}

	var status string
	switch action {
	case "create":
		tag, insertErr := tx.Exec(ctx, `
   INSERT INTO lessons (id,tenant_id,offering_id,author_membership_id,curriculum_selection_id,primary_objective_id,title,content)
   SELECT $1,o.tenant_id,o.id,$4,s.id,p.id,$6,$7
   FROM subject_offerings o
   JOIN academic_terms term ON term.tenant_id=o.tenant_id AND term.id=o.term_id
   JOIN classes c ON c.tenant_id=o.tenant_id AND c.session_id=term.session_id AND c.id=o.class_id
   JOIN grade_levels g ON g.tenant_id=c.tenant_id AND g.id=c.grade_level_id
   JOIN school_current_curriculum_selections s ON s.tenant_id=o.tenant_id AND s.session_id=term.session_id
    AND s.stage=g.stage AND s.curriculum_role='PRIMARY'
   JOIN curriculum_objectives p ON p.id=$5 AND p.framework_version_id=s.framework_version_id AND p.stage=s.stage
   WHERE o.tenant_id=$2 AND o.id=$3
  `, id, tenant, draft.OfferingID, membership, draft.ObjectiveID, draft.Title, draft.Content)
		err = insertErr
		if err == nil && tag.RowsAffected() != 1 {
			invalidLesson(w, r)
			return
		}
		status = "DRAFT"
	default:
		var before string
		var revision int64
		err = tx.QueryRow(ctx, `SELECT status,content_revision FROM lessons WHERE tenant_id=$1 AND id=$2 AND author_membership_id=$3 FOR UPDATE`, tenant, id, membership).Scan(&before, &revision)
		if err != nil {
			teacherLessonError(w, r, err)
			return
		}
		switch {
		case action == "edit" && before == "DRAFT":
			_, err = tx.Exec(ctx, `UPDATE lessons SET title=$3,content=$4 WHERE tenant_id=$1 AND id=$2`, tenant, id, draft.Title, draft.Content)
			status = "DRAFT"
		case action == "submit" && before == "DRAFT":
			var tag pgconn.CommandTag
			tag, err = tx.Exec(ctx, `INSERT INTO lesson_reviews (tenant_id,lesson_id,lesson_revision)
				VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, tenant, id, revision)
			if err == nil && tag.RowsAffected() == 0 {
				err = errLessonConflict
			}
			status = "DRAFT"
		case action == "publish" && before == "DRAFT":
			status = "PUBLISHED"
		case action == "archive" && before == "PUBLISHED":
			status = "ARCHIVED"
		default:
			err = errLessonConflict
		}
		if err == nil && (action == "publish" || action == "archive") {
			_, err = tx.Exec(ctx, `UPDATE lessons SET status=$3 WHERE tenant_id=$1 AND id=$2`, tenant, id, status)
		}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id)
   VALUES ($1,$2,$3,$4,'lesson',$5,$6)`, uuid.New(), tenant, actor.ID, "learning.lesson."+action, id.String(), middleware.GetRequestID(ctx))
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	code := http.StatusOK
	if action == "create" {
		code = http.StatusCreated
	}
	response.JSON(w, r, code, map[string]any{"id": id, "status": status})
}

func decodeLesson(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 160<<10))
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst) == nil && errors.Is(decoder.Decode(new(any)), io.EOF)
}
func validLessonText(title, content *string) bool {
	*title = strings.TrimSpace(*title)
	*content = strings.TrimSpace(*content)
	return *title != "" && *content != "" && utf8.RuneCountInString(*title) <= 160 && utf8.RuneCountInString(*content) <= 24000 &&
		!strings.ContainsRune(*title, 0) && !strings.ContainsRune(*content, 0)
}
func invalidLesson(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid lesson text or curriculum objective", nil)
}
func teacherLessonError(w http.ResponseWriter, r *http.Request, err error) {
	var constraint *pgconn.PgError
	if errors.Is(err, errLessonConflict) || (errors.As(err, &constraint) && constraint.Code == "23514") {
		response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "Lesson state or curriculum is no longer eligible", nil)
		return
	}
	lessonReadError(w, r, err)
}
