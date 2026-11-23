package learning

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

type submissionReceipt struct {
	Revision    int32     `json:"revision"`
	SubmittedAt time.Time `json:"submitted_at"`
}

const studentSubmissionScope = studentLessonJoins + assignmentJoin + studentLessonEligibility + `
	AND l.id=$3 AND a1.id=$4 AND a1.status IN ('OPEN','CLOSED')
`

func (h *Handler) SubmitAssignment(w http.ResponseWriter, r *http.Request) {
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	lesson, ok := teachingRouteID(w, r, "lessonID")
	if !ok {
		return
	}
	assignment, ok := teachingRouteID(w, r, "assignmentID")
	if !ok {
		return
	}
	var input struct {
		RequestID uuid.UUID `json:"request_id"`
		Content   string    `json:"content"`
	}
	if !decodeLesson(w, r, &input) || input.RequestID == uuid.Nil {
		invalidSubmission(w, r)
		return
	}
	input.Content = strings.TrimSpace(input.Content)
	if input.Content == "" || utf8.RuneCountInString(input.Content) > 24000 || strings.ContainsRune(input.Content, 0) {
		invalidSubmission(w, r)
		return
	}
	h.recordAssignmentSubmission(w, r, actor, lesson, assignment, input.RequestID, "TEXT", &input.Content, nil, nil)
}

func (h *Handler) recordAssignmentSubmission(w http.ResponseWriter, r *http.Request, actor, lesson, assignment, requestID uuid.UUID,
	expectedKind string, content *string, fileID *uuid.UUID, checksum *string) {
	ctx := r.Context()
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	slug := chi.URLParam(r, "slug")
	var tenant, student uuid.UUID
	// Keep closure and lesson archival outside the acceptance transaction.
	err = tx.QueryRow(ctx, `SELECT t.id,s.id`+studentSubmissionScope+` LIMIT 1 FOR SHARE OF l,a1`,
		slug, actor, lesson, assignment).Scan(&tenant, &student)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	// Serialize attempts by one learner while leaving different learners free to submit.
	err = tx.QueryRow(ctx, `SELECT id FROM students WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenant, student).Scan(&student)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	var status, kind string
	var due, now time.Time
	var retry bool
	err = tx.QueryRow(ctx, `SELECT a1.status,a1.submission_type,a1.due_at,a1.allow_resubmission,clock_timestamp()`+
		studentSubmissionScope+` AND s.id=$5 LIMIT 1`, slug, actor, lesson, assignment, student).Scan(&status, &kind, &due, &retry, &now)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}

	var receipt submissionReceipt
	var previousContent, previousChecksum *string
	err = tx.QueryRow(ctx, `SELECT revision,submitted_at,content,file_checksum FROM assignment_submissions
		WHERE tenant_id=$1 AND assignment_id=$2 AND student_id=$3 AND request_id=$4`,
		tenant, assignment, student, requestID).Scan(&receipt.Revision, &receipt.SubmittedAt, &previousContent, &previousChecksum)
	if err == nil {
		if !sameSubmissionValue(content, previousContent) || !sameSubmissionValue(checksum, previousChecksum) {
			submissionUnavailable(w, r)
			return
		}
		response.JSON(w, r, http.StatusOK, receipt)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		lessonReadError(w, r, err)
		return
	}
	if status != "OPEN" || kind != expectedKind || !now.Before(due) {
		submissionUnavailable(w, r)
		return
	}

	var revision int32
	err = tx.QueryRow(ctx, `SELECT COALESCE(max(revision),0) FROM assignment_submissions
		WHERE tenant_id=$1 AND assignment_id=$2 AND student_id=$3`, tenant, assignment, student).Scan(&revision)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	if revision > 0 && !retry {
		submissionUnavailable(w, r)
		return
	}
	id := uuid.New()
	err = tx.QueryRow(ctx, `INSERT INTO assignment_submissions
		(id,tenant_id,assignment_id,student_id,request_id,revision,content,submitted_at,file_id,file_checksum)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING revision,submitted_at`,
		id, tenant, assignment, student, requestID, revision+1, content, now, fileID, checksum).Scan(&receipt.Revision, &receipt.SubmittedAt)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id)
			VALUES ($1,$2,$3,'learning.assignment.submit','assignment_submission',$4,$5)`,
			uuid.New(), tenant, actor, id.String(), middleware.GetRequestID(ctx))
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, receipt)
}

func sameSubmissionValue(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func invalidSubmission(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Enter a valid text submission", nil)
}
func submissionUnavailable(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "This assignment is no longer accepting this submission", nil)
}
