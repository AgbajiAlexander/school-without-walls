package learning

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/platform/media"
	"github.com/school-without-walls/api/internal/platform/response"
)

const maxSubmissionFileBytes = 4 << 20

func (h *Handler) UploadAssignmentFile(w http.ResponseWriter, r *http.Request) {
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
	requestID, err := uuid.Parse(r.Header.Get("Idempotency-Key"))
	if err != nil || requestID == uuid.Nil {
		invalidSubmissionFile(w, r)
		return
	}
	ctx := r.Context()
	var tenant, student uuid.UUID
	var canUpload bool
	var oldChecksum *string
	var receipt submissionReceipt
	slug := chi.URLParam(r, "slug")
	err = h.pool.QueryRow(ctx, `SELECT t.id,s.id,(a1.status='OPEN' AND clock_timestamp()<a1.due_at
		AND (a1.allow_resubmission OR NOT EXISTS (SELECT 1 FROM assignment_submissions other
		WHERE other.tenant_id=a1.tenant_id AND other.assignment_id=a1.id AND other.student_id=s.id))),
		prior.file_checksum,COALESCE(prior.revision,0),COALESCE(prior.submitted_at,statement_timestamp())`+
		studentLessonJoins+assignmentJoin+`
		LEFT JOIN assignment_submissions prior ON prior.tenant_id=a1.tenant_id AND prior.assignment_id=a1.id
			AND prior.student_id=s.id AND prior.request_id=$5`+studentLessonEligibility+`
		AND l.id=$3 AND a1.id=$4 AND a1.submission_type='FILE' AND a1.status IN ('OPEN','CLOSED') LIMIT 1`,
		slug, actor, lesson, assignment, requestID).Scan(&tenant, &student, &canUpload, &oldChecksum, &receipt.Revision, &receipt.SubmittedAt)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	if !canUpload && oldChecksum == nil {
		submissionUnavailable(w, r)
		return
	}
	select {
	case h.scanSlots <- struct{}{}:
		defer func() { <-h.scanSlots }()
	default:
		response.ErrorJSON(w, r, http.StatusTooManyRequests, response.ErrCodeRateLimited, "Media scanning is busy. Try again shortly.", nil)
		return
	}
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSubmissionFileBytes))
	if err != nil || media.Validate(payload, len(payload), r.Header.Get("Content-Type")) != nil {
		invalidSubmissionFile(w, r)
		return
	}
	sum := sha256.Sum256(payload)
	checksum := base64.StdEncoding.EncodeToString(sum[:])
	if oldChecksum != nil {
		if *oldChecksum != checksum {
			submissionUnavailable(w, r)
			return
		}
		response.JSON(w, r, http.StatusOK, receipt)
		return
	}

	object := media.Object{TenantID: tenant, ID: uuid.New(), Type: r.Header.Get("Content-Type"), Size: len(payload), Checksum: checksum}
	if err = h.mediaStore.StoreScannedUpload(ctx, object, payload, h.mediaScanner); err != nil {
		mediaFailure(w, r, err)
		return
	}
	h.recordAssignmentSubmission(w, r, actor, lesson, assignment, requestID, "FILE", nil, &object.ID, &checksum)
}

func (h *Handler) DownloadAssignmentFile(w http.ResponseWriter, r *http.Request) {
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
	var tenant, object uuid.UUID
	err := h.pool.QueryRow(r.Context(), `SELECT t.id,sub.file_id`+reviewSubmissions+`
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND l.id=$3 AND a1.id=$4
		AND sub.id=$5 AND sub.file_id IS NOT NULL AND `+latestSubmission,
		chi.URLParam(r, "slug"), actor, lesson, assignment, submission).Scan(&tenant, &object)
	if errors.Is(err, pgx.ErrNoRows) {
		lessonUnavailable(w, r)
		return
	}
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	h.writeDownloadGrant(w, r, tenant, object)
}

func invalidSubmissionFile(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Choose a PDF, PNG or JPEG file up to 4 MiB", nil)
}
