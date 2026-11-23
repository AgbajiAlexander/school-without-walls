package learning

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

type assignmentDraft struct {
	Title             string    `json:"title"`
	Instructions      string    `json:"instructions"`
	DueAt             time.Time `json:"due_at"`
	MaxScore          int32     `json:"max_score"`
	SubmissionType    string    `json:"submission_type"`
	AllowResubmission bool      `json:"allow_resubmission"`
}

type assignmentRecord struct {
	ID uuid.UUID `json:"id"`
	assignmentDraft
	Status string `json:"status"`
}

const assignmentFields = `a1.id,a1.title,a1.instructions,a1.due_at,a1.max_score,a1.submission_type,a1.allow_resubmission,a1.status`
const teacherAssignmentLesson = teacherOfferingJoins + `
	JOIN lessons l ON l.tenant_id=o.tenant_id AND l.offering_id=o.id AND l.author_membership_id=m.id
`
const assignmentJoin = ` JOIN assignments a1 ON a1.tenant_id=l.tenant_id AND a1.lesson_id=l.id `

func (h *Handler) ListTeacherAssignments(w http.ResponseWriter, r *http.Request) {
	id, ok := teachingRouteID(w, r, "lessonID")
	if !ok {
		return
	}
	teacherPage(h, w, r, pgx.RowToStructByName[assignmentRecord], func(row assignmentRecord) uuid.UUID { return row.ID },
		`SELECT `+assignmentFields+teacherAssignmentLesson+assignmentJoin+`
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND l.id=$4
		  AND ($3::uuid IS NULL OR a1.id>$3) ORDER BY a1.id LIMIT 51`, id)
}

func (h *Handler) ListStudentAssignments(w http.ResponseWriter, r *http.Request) {
	id, ok := teachingRouteID(w, r, "lessonID")
	if !ok {
		return
	}
	type learnerAssignment struct {
		FileUploadEnabled bool `json:"file_upload_enabled,omitempty"`
		assignmentRecord
		Feedback   *returnedFeedback  `json:"feedback,omitempty"`
		Submission *submissionReceipt `json:"submission,omitempty"`
	}
	teacherPage(h, w, r, pgx.RowToStructByName[learnerAssignment], func(row learnerAssignment) uuid.UUID { return row.ID },
		`SELECT DISTINCT `+assignmentFields+`, ($5::boolean AND a1.submission_type='FILE') AS file_upload_enabled, (
			SELECT jsonb_build_object('revision',sub.revision,'submitted_at',sub.submitted_at)
			FROM assignment_submissions sub
			WHERE sub.tenant_id=l.tenant_id AND sub.assignment_id=a1.id AND sub.student_id=s.id
			ORDER BY sub.revision DESC LIMIT 1
		) AS submission, (SELECT `+feedbackJSON+`
			FROM assignment_submissions sub`+feedbackJoin+`
			WHERE sub.tenant_id=l.tenant_id AND sub.assignment_id=a1.id AND sub.student_id=s.id
			ORDER BY sub.revision DESC LIMIT 1) AS feedback`+studentLessonJoins+assignmentJoin+studentLessonEligibility+`
		AND l.id=$4 AND a1.status IN ('OPEN','CLOSED')
		AND ($3::uuid IS NULL OR a1.id>$3) ORDER BY a1.id LIMIT 51`, id, h.mediaStore != nil)
}

func (h *Handler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	h.writeAssignment(w, r, "create")
}

func (h *Handler) EditAssignment(w http.ResponseWriter, r *http.Request) {
	h.writeAssignment(w, r, "edit")
}

func (h *Handler) OpenAssignment(w http.ResponseWriter, r *http.Request) {
	h.writeAssignment(w, r, "open")
}

func (h *Handler) CloseAssignment(w http.ResponseWriter, r *http.Request) {
	h.writeAssignment(w, r, "close")
}

func (h *Handler) writeAssignment(w http.ResponseWriter, r *http.Request, action string) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	lesson, ok := teachingRouteID(w, r, "lessonID")
	if !ok {
		return
	}
	var assignment uuid.UUID
	if action == "create" {
		assignment = uuid.New()
	} else {
		assignment, ok = teachingRouteID(w, r, "assignmentID")
		if !ok {
			return
		}
	}
	var draft assignmentDraft
	if action == "create" || action == "edit" {
		if !decodeLesson(w, r, &draft) || !validLessonText(&draft.Title, &draft.Instructions) ||
			draft.DueAt.IsZero() || draft.MaxScore < 1 || (draft.SubmissionType != "TEXT" && draft.SubmissionType != "FILE") {
			response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Check the assignment details", nil)
			return
		}
	}
	ctx := r.Context()
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)

	var tenant uuid.UUID
	err = tx.QueryRow(ctx, `SELECT t.id`+teacherAssignmentLesson+`
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND l.id=$3 AND l.status<>'ARCHIVED'
		FOR SHARE OF t,u,m,a,o,l`, chi.URLParam(r, "slug"), actor, lesson).Scan(&tenant)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}

	var status string
	if action == "create" {
		err = tx.QueryRow(ctx, `INSERT INTO assignments
			(id,tenant_id,lesson_id,title,instructions,due_at,max_score,submission_type,allow_resubmission)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING status`,
			assignment, tenant, lesson, draft.Title, draft.Instructions, draft.DueAt, draft.MaxScore, draft.SubmissionType, draft.AllowResubmission).Scan(&status)
	} else {
		var kind string
		err = tx.QueryRow(ctx, `SELECT status,submission_type FROM assignments WHERE tenant_id=$1 AND lesson_id=$2 AND id=$3 FOR UPDATE`,
			tenant, lesson, assignment).Scan(&status, &kind)
		if err == nil {
			switch {
			case action == "edit" && status == "DRAFT":
				_, err = tx.Exec(ctx, `UPDATE assignments SET title=$4,instructions=$5,due_at=$6,max_score=$7,submission_type=$8,allow_resubmission=$9
					WHERE tenant_id=$1 AND lesson_id=$2 AND id=$3`, tenant, lesson, assignment,
					draft.Title, draft.Instructions, draft.DueAt, draft.MaxScore, draft.SubmissionType, draft.AllowResubmission)
			case action == "open" && kind == "FILE" && h.mediaStore == nil:
				err = errLessonConflict
			case (action == "open" && status == "DRAFT") || (action == "close" && status == "OPEN"):
				status = strings.ToUpper(action)
				if action == "close" {
					status = "CLOSED"
				}
				_, err = tx.Exec(ctx, `UPDATE assignments SET status=$4 WHERE tenant_id=$1 AND lesson_id=$2 AND id=$3`, tenant, lesson, assignment, status)
			default:
				err = errLessonConflict
			}
		}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id)
			VALUES ($1,$2,$3,$4,'assignment',$5,$6)`, uuid.New(), tenant, actor, "learning.assignment."+action, assignment.String(), middleware.GetRequestID(ctx))
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
	response.JSON(w, r, code, map[string]any{"id": assignment, "status": status})
}
