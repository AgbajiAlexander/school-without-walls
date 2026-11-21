package learning

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/platform/media"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

func (h *Handler) ConfigureMedia(store *media.Store, scanner media.Scanner) {
	h.mediaStore = store
	h.mediaScanner = scanner
	h.scanSlots = make(chan struct{}, 4)
}

func (h *Handler) UploadLessonMedia(w http.ResponseWriter, r *http.Request) {
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	lesson, ok := teachingRouteID(w, r, "lessonID")
	if !ok {
		return
	}
	var object media.Object
	if !decodeLesson(w, r, &struct {
		Type     *string `json:"media_type"`
		Size     *int    `json:"size_bytes"`
		Checksum *string `json:"checksum_sha256"`
	}{&object.Type, &object.Size, &object.Checksum}) {
		invalidLesson(w, r)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	tenant, err := lockMediaLesson(ctx, tx, chi.URLParam(r, "slug"), actor, lesson)
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	object.TenantID, object.ID = tenant, uuid.New()
	signed, err := h.mediaStore.SignUpload(ctx, object)
	if err != nil {
		mediaFailure(w, r, err)
		return
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO lesson_media (id,tenant_id,lesson_id,media_type,size_bytes,checksum_sha256)
		SELECT $1,$2,$3,$4,$5,$6 WHERE (
			SELECT count(*) FROM lesson_media WHERE tenant_id=$2 AND lesson_id=$3
		)<20
	`, object.ID, tenant, lesson, object.Type, object.Size, object.Checksum)
	if err == nil && tag.RowsAffected() == 0 {
		response.ErrorJSON(w, r, http.StatusTooManyRequests, response.ErrCodeRateLimited, "Lesson media upload limit reached", nil)
		return
	}
	if err == nil {
		err = writeMediaAudit(ctx, tx, r, tenant, actor, object.ID, "learning.media.upload_intent")
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{"id": object.ID, "upload": signed, "expires_in": int(media.URLLifetime.Seconds())})
}

func (h *Handler) CompleteLessonMedia(w http.ResponseWriter, r *http.Request) {
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	lesson, ok := teachingRouteID(w, r, "lessonID")
	if !ok {
		return
	}
	id, ok := teachingRouteID(w, r, "mediaID")
	if !ok {
		return
	}
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")
	object := media.Object{ID: id}
	var state string
	err := h.pool.QueryRow(ctx, `
		SELECT item.tenant_id,item.media_type,item.size_bytes,item.checksum_sha256,item.status`+teacherOfferingJoins+`
		JOIN lessons l ON l.tenant_id=o.tenant_id AND l.offering_id=o.id AND l.author_membership_id=m.id AND l.status='DRAFT'
		JOIN lesson_media item ON item.tenant_id=l.tenant_id AND item.lesson_id=l.id
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND l.id=$3 AND item.id=$4
		  AND item.checksum_sha256 IS NOT NULL AND (item.status='READY' OR
		    (item.status='PENDING' AND item.created_at > statement_timestamp()-interval '15 minutes'))
	`, slug, actor, lesson, id).Scan(&object.TenantID, &object.Type, &object.Size, &object.Checksum, &state)
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	if state == "READY" {
		response.JSON(w, r, http.StatusOK, map[string]any{"id": id, "status": "READY"})
		return
	}

	select {
	case h.scanSlots <- struct{}{}:
		defer func() { <-h.scanSlots }()
	default:
		response.ErrorJSON(w, r, http.StatusTooManyRequests, response.ErrCodeRateLimited, "Media scanning is busy. Try again shortly.", nil)
		return
	}
	scanErr := h.mediaStore.ScanAndPromote(ctx, object, h.mediaScanner)
	rejected := errors.Is(scanErr, media.ErrInvalidObject) || errors.Is(scanErr, media.ErrInfected)
	if scanErr != nil && !rejected {
		mediaFailure(w, r, scanErr)
		return
	}

	// Do not hold database locks during external scanning. Reauthorize before
	// changing state and attaching the scanned object to the lesson.
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	tenant, err := lockMediaLesson(ctx, tx, slug, actor, lesson)
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	if tenant != object.TenantID {
		lessonUnavailable(w, r)
		return
	}
	next := "READY"
	if rejected {
		next = "REJECTED"
	}
	tag, err := tx.Exec(ctx, `UPDATE lesson_media SET status=$4
		WHERE tenant_id=$1 AND lesson_id=$2 AND id=$3 AND status='PENDING'
		  AND created_at > statement_timestamp()-interval '15 minutes'`, tenant, lesson, id, next)
	if err == nil && tag.RowsAffected() != 1 {
		err = errLessonConflict
	}
	if err == nil && !rejected {
		_, err = tx.Exec(ctx, `UPDATE lessons SET media_id=$3 WHERE tenant_id=$1 AND id=$2`, tenant, lesson, id)
	}
	if err == nil {
		err = writeMediaAudit(ctx, tx, r, tenant, actor, id, "learning.media."+next)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	if rejected {
		mediaFailure(w, r, scanErr)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"id": id, "status": next})
}

func lockMediaLesson(ctx context.Context, tx pgx.Tx, slug string, actor, lesson uuid.UUID) (uuid.UUID, error) {
	var offering uuid.UUID
	err := tx.QueryRow(ctx, `SELECT l.offering_id FROM lessons l JOIN tenants t ON t.id=l.tenant_id
		JOIN memberships m ON m.tenant_id=l.tenant_id AND m.id=l.author_membership_id
		WHERE t.slug=$1::citext AND l.id=$2 AND m.user_id=$3 AND l.status='DRAFT'`, slug, lesson, actor).Scan(&offering)
	if err != nil {
		return uuid.Nil, err
	}
	var tenant, membership uuid.UUID
	err = tx.QueryRow(ctx, `SELECT t.id,m.id`+teacherOffering+` FOR SHARE OF t,u,m,a,o`, slug, actor, offering).Scan(&tenant, &membership)
	if err != nil {
		return uuid.Nil, err
	}
	var locked uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM lessons WHERE tenant_id=$1 AND id=$2 AND author_membership_id=$3 AND status='DRAFT'
		FOR UPDATE`, tenant, lesson, membership).Scan(&locked)
	return tenant, err
}

func writeMediaAudit(ctx context.Context, tx pgx.Tx, r *http.Request, tenant, actor, id uuid.UUID, action string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id)
		VALUES ($1,$2,$3,$4,'lesson_media',$5,$6)`, uuid.New(), tenant, actor, action, id.String(), middleware.GetRequestID(r.Context()))
	return err
}

func (h *Handler) DownloadLessonMedia(w http.ResponseWriter, r *http.Request) {
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	id, ok := teachingRouteID(w, r, "lessonID")
	if !ok {
		return
	}
	var tenant, mediaID uuid.UUID
	err := h.pool.QueryRow(r.Context(), `SELECT l.tenant_id,l.media_id`+studentLessonScope+`
		AND EXISTS (SELECT 1 FROM lesson_media item WHERE item.tenant_id=l.tenant_id
			AND item.lesson_id=l.id AND item.id=l.media_id AND item.status='READY' AND item.checksum_sha256 IS NOT NULL)
	`, chi.URLParam(r, "slug"), actor, id, nil).Scan(&tenant, &mediaID)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	signed, err := h.mediaStore.SignDownload(r.Context(), tenant, mediaID)
	if err != nil {
		mediaFailure(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"download": signed, "expires_in": int(media.URLLifetime.Seconds())})
}

func mediaFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, media.ErrInvalidObject) || errors.Is(err, media.ErrInfected) {
		response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "The media did not pass validation", nil)
		return
	}
	response.ErrorJSON(w, r, http.StatusServiceUnavailable, response.ErrCodeServiceUnavailable, "Media processing is temporarily unavailable", nil)
}
