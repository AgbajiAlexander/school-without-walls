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

type returnedFeedback struct {
	Score   int32  `json:"score"`
	Comment string `json:"comment"`
	Version int32  `json:"version"`
}

type submissionForReview struct {
	HasFile     bool              `json:"has_file,omitempty"`
	ID          uuid.UUID         `json:"id"`
	StudentName string            `json:"student_name"`
	Content     *string           `json:"content"`
	Revision    int32             `json:"revision"`
	SubmittedAt time.Time         `json:"submitted_at"`
	Feedback    *returnedFeedback `json:"feedback,omitempty"`
}

const reviewSubmissions = teacherAssignmentLesson + assignmentJoin + `
	JOIN assignment_submissions sub ON sub.tenant_id=a1.tenant_id AND sub.assignment_id=a1.id
	JOIN students learner ON learner.tenant_id=sub.tenant_id AND learner.id=sub.student_id
`
const latestSubmission = `NOT EXISTS (SELECT 1 FROM assignment_submissions newer
	WHERE newer.tenant_id=sub.tenant_id AND newer.assignment_id=sub.assignment_id
	AND newer.student_id=sub.student_id AND newer.revision>sub.revision)`
const feedbackJoin = ` LEFT JOIN assignment_feedback f ON f.tenant_id=sub.tenant_id AND f.submission_id=sub.id `
const feedbackJSON = `CASE WHEN f.submission_id IS NOT NULL THEN
	jsonb_build_object('score',f.score,'comment',f.comment,'version',f.version) END`

func (h *Handler) ListSubmissionsForReview(w http.ResponseWriter, r *http.Request) {
	lesson, ok := teachingRouteID(w, r, "lessonID")
	if !ok {
		return
	}
	assignment, ok := teachingRouteID(w, r, "assignmentID")
	if !ok {
		return
	}
	teacherPage(h, w, r, pgx.RowToStructByName[submissionForReview], func(row submissionForReview) uuid.UUID { return row.ID },
		`SELECT sub.id,learner.display_name AS student_name,sub.content,sub.revision,sub.submitted_at,($6::boolean AND sub.file_id IS NOT NULL) AS has_file,`+feedbackJSON+` AS feedback`+
			reviewSubmissions+feedbackJoin+` WHERE t.slug=$1::citext AND t.status='ACTIVE' AND l.id=$4 AND a1.id=$5
		AND `+latestSubmission+` AND ($3::uuid IS NULL OR sub.id>$3) ORDER BY sub.id LIMIT 51`, lesson, assignment, h.mediaStore != nil)
}

func (h *Handler) ReturnAssignmentFeedback(w http.ResponseWriter, r *http.Request) {
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
	submission, ok := teachingRouteID(w, r, "submissionID")
	if !ok {
		return
	}
	var input struct {
		Score   *int32 `json:"score"`
		Comment string `json:"comment"`
		Version *int32 `json:"version"`
	}
	if !decodeLesson(w, r, &input) {
		invalidFeedback(w, r)
		return
	}
	input.Comment = strings.TrimSpace(input.Comment)
	if input.Score == nil || *input.Score < 0 || input.Version == nil || *input.Version < 0 ||
		input.Comment == "" || utf8.RuneCountInString(input.Comment) > 4000 || strings.ContainsRune(input.Comment, 0) {
		invalidFeedback(w, r)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var tenant, reviewer, student uuid.UUID
	var maxScore int32
	err = tx.QueryRow(ctx, `SELECT t.id,m.id,learner.id,a1.max_score`+reviewSubmissions+`
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND l.id=$3 AND a1.id=$4 AND sub.id=$5
		FOR SHARE OF t,u,m,a,o,l,a1`, chi.URLParam(r, "slug"), actor, lesson, assignment, submission).Scan(&tenant, &reviewer, &student, &maxScore)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	if *input.Score > maxScore {
		invalidFeedback(w, r)
		return
	}
	// Both feedback and submission writers take the learner lock after the lesson and assignment.
	err = tx.QueryRow(ctx, `SELECT id FROM students WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenant, student).Scan(&student)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	var current bool
	err = tx.QueryRow(ctx, `SELECT `+latestSubmission+` FROM assignment_submissions sub
		WHERE sub.tenant_id=$1 AND sub.id=$2`, tenant, submission).Scan(&current)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	if !current {
		staleFeedback(w, r)
		return
	}

	var previousScore *int32
	var version int32
	err = tx.QueryRow(ctx, `SELECT score,version FROM assignment_feedback WHERE tenant_id=$1 AND submission_id=$2`,
		tenant, submission).Scan(&previousScore, &version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		lessonReadError(w, r, err)
		return
	}
	if version != *input.Version {
		staleFeedback(w, r)
		return
	}
	feedback := returnedFeedback{Score: *input.Score, Comment: input.Comment, Version: version + 1}
	_, err = tx.Exec(ctx, `INSERT INTO assignment_feedback (tenant_id,submission_id,reviewer_id,score,comment,version)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (tenant_id,submission_id) DO UPDATE
		SET reviewer_id=EXCLUDED.reviewer_id,score=EXCLUDED.score,comment=EXCLUDED.comment,
		version=EXCLUDED.version,returned_at=clock_timestamp()`, tenant, submission, reviewer, feedback.Score, feedback.Comment, feedback.Version)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id,metadata_redacted)
			VALUES ($1,$2,$3,'learning.assignment.feedback','assignment_submission',$4,$5,
				jsonb_build_object('previous_score',$6::integer,'score',$7::integer,'version',$8::integer))`,
			uuid.New(), tenant, actor, submission.String(), middleware.GetRequestID(ctx), previousScore, feedback.Score, feedback.Version)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, feedback)
}

func invalidFeedback(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Enter a score within range and a feedback comment", nil)
}

func staleFeedback(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "The submission or its feedback changed. Refresh and review again.", nil)
}
