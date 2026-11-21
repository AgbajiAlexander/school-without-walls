package learning

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

const lessonReviewerScope = `
	FROM tenants t JOIN users u ON u.id=$2 AND u.status='ACTIVE'
	JOIN memberships m ON m.tenant_id=t.id AND m.user_id=u.id AND m.status='ACTIVE'
	  AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN','PRINCIPAL')
	WHERE t.slug=$1::citext AND t.status='ACTIVE'
`

type pendingLessonReview struct {
	ID         uuid.UUID          `json:"id"`
	Title      string             `json:"title"`
	Content    string             `json:"content"`
	Revision   int64              `json:"revision"`
	Enrichment []lessonEnrichment `json:"enrichment,omitempty"`
}

func (h *Handler) ListLessonReviews(w http.ResponseWriter, r *http.Request) {
	teacherPage(h, w, r, pgx.RowToStructByName[pendingLessonReview], func(item pendingLessonReview) uuid.UUID { return item.ID },
		`SELECT l.id,l.title,l.content,l.content_revision AS revision,`+lessonEnrichmentDetails+` AS enrichment FROM lessons l
		JOIN lesson_reviews review ON review.tenant_id=l.tenant_id AND review.lesson_id=l.id AND review.lesson_revision=l.content_revision
		JOIN memberships author ON author.tenant_id=l.tenant_id AND author.id=l.author_membership_id
		WHERE l.status='DRAFT' AND author.user_id<>$2 AND review.approved_at IS NULL
		  AND EXISTS (SELECT 1`+lessonReviewerScope+` AND t.id=l.tenant_id)
		  AND ($3::uuid IS NULL OR l.id>$3) ORDER BY l.id LIMIT 51`)
}

func (h *Handler) ApproveLesson(w http.ResponseWriter, r *http.Request) {
	id, ok := teachingRouteID(w, r, "lessonID")
	if !ok {
		return
	}
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	var request struct {
		Revision int64 `json:"revision"`
	}
	if !decodeLesson(w, r, &request) || request.Revision <= 0 {
		invalidLesson(w, r)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var tenant uuid.UUID
	err = tx.QueryRow(ctx, `SELECT t.id`+lessonReviewerScope+` ORDER BY m.id LIMIT 1 FOR SHARE OF t,u,m`,
		chi.URLParam(r, "slug"), actor).Scan(&tenant)
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	var revision int64
	err = tx.QueryRow(ctx, `SELECT l.content_revision FROM lessons l
		JOIN memberships author ON author.tenant_id=l.tenant_id AND author.id=l.author_membership_id
		WHERE l.tenant_id=$1 AND l.id=$2 AND l.status='DRAFT' AND author.user_id<>$3 FOR UPDATE OF l`,
		tenant, id, actor).Scan(&revision)
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	if revision != request.Revision {
		teacherLessonError(w, r, errLessonConflict)
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE lesson_reviews SET reviewer_id=$4,approved_at=clock_timestamp()
		WHERE tenant_id=$1 AND lesson_id=$2 AND lesson_revision=$3 AND approved_at IS NULL`, tenant, id, revision, actor)
	if err == nil && tag.RowsAffected() != 1 {
		err = errLessonConflict
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id)
			VALUES ($1,$2,$3,'learning.lesson.approve','lesson',$4,$5)`, uuid.New(), tenant, actor, id.String(), middleware.GetRequestID(ctx))
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"id": id, "review_status": "APPROVED"})
}

func (h *Handler) SetLessonReviewPolicy(w http.ResponseWriter, r *http.Request) {
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	var input struct {
		Required *bool `json:"required"`
	}
	if !decodeLesson(w, r, &input) || input.Required == nil {
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
	var tenant uuid.UUID
	err = tx.QueryRow(ctx, `SELECT t.id FROM tenants t
		JOIN memberships m ON m.tenant_id=t.id AND m.user_id=$2 AND m.status='ACTIVE' AND m.role IN ('SCHOOL_OWNER','SCHOOL_ADMIN')
		JOIN users u ON u.id=m.user_id AND u.status='ACTIVE'
		WHERE t.slug=$1::citext AND t.status='ACTIVE'
		ORDER BY m.id LIMIT 1 FOR UPDATE OF t FOR SHARE OF u,m`, chi.URLParam(r, "slug"), actor).Scan(&tenant)
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	_, err = tx.Exec(ctx, `UPDATE tenants SET feature_flags=jsonb_set(feature_flags,'{lesson_review_required}',to_jsonb($2::boolean)),
		updated_at=clock_timestamp() WHERE id=$1`, tenant, *input.Required)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id,metadata_redacted)
			VALUES ($1,$2,$3,'learning.lesson_review.policy','tenant',$4,$5,jsonb_build_object('required',$6::boolean))`,
			uuid.New(), tenant, actor, tenant.String(), middleware.GetRequestID(ctx), *input.Required)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		teacherLessonError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"required": *input.Required})
}
