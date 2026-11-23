package learning

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/response"
)

type teachingOfferingSummary struct {
	ID        uuid.UUID `json:"id"`
	Subject   string    `json:"subject"`
	ClassName string    `json:"class_name"`
	ArmName   *string   `json:"arm_name"`
	TermName  string    `json:"term_name"`
}

type teachingObjective struct {
	ID            uuid.UUID          `json:"id"`
	Code          string             `json:"code"`
	ObjectiveText string             `json:"objective_text"`
	Enrichment    []enrichmentOption `json:"enrichment"`
}

type teacherLessonRecord struct {
	FileSubmissionEnabled bool               `json:"file_submission_enabled,omitempty"`
	MediaUploadEnabled    bool               `json:"media_upload_enabled,omitempty"`
	ID                    uuid.UUID          `json:"id"`
	OfferingID            uuid.UUID          `json:"offering_id"`
	Title                 string             `json:"title"`
	Status                string             `json:"status"`
	Content               *string            `json:"content,omitempty"`
	ObjectiveCode         string             `json:"objective_code"`
	ReviewRequired        bool               `json:"review_required"`
	ReviewStatus          string             `json:"review_status"`
	Enrichment            []enrichmentOption `json:"enrichment,omitempty"`
}

type enrichmentOption struct {
	ID       uuid.UUID `json:"id"`
	Code     string    `json:"code"`
	Text     string    `json:"text"`
	Selected bool      `json:"selected"`
	Eligible bool      `json:"eligible"`
}

func teachingEnrichment(session, selected string) string {
	return `(SELECT COALESCE(jsonb_agg(jsonb_build_object(
		'id',a.id,'code',related.code,'text',related.objective_text,
		'selected',a.id=ANY(` + selected + `),'eligible',choice.id IS NOT NULL AND related_edition.status='PUBLISHED'
	) ORDER BY related.code,a.id),'[]'::jsonb)
	FROM curriculum_objective_alignments a
	JOIN curriculum_objectives related ON related.id=a.related_objective_id AND related.stage=p.stage
	JOIN curriculum_framework_versions related_edition ON related_edition.id=related.framework_version_id
	LEFT JOIN school_current_curriculum_selections choice ON choice.tenant_id=t.id
		AND choice.session_id=` + session + ` AND choice.stage=p.stage AND choice.curriculum_role='ENRICHMENT'
		AND choice.framework_version_id=related.framework_version_id
	WHERE a.primary_objective_id=p.id AND a.review_status='HUMAN_REVIEWED'
	  AND ((choice.id IS NOT NULL AND related_edition.status='PUBLISHED') OR a.id=ANY(` + selected + `))) AS enrichment`
}

func (h *Handler) ListTeachingOfferings(w http.ResponseWriter, r *http.Request) {
	teacherPage(h, w, r, pgx.RowToStructByName[teachingOfferingSummary], func(o teachingOfferingSummary) uuid.UUID { return o.ID },
		`SELECT o.id, subject.name AS subject, c.name AS class_name, arm.name AS arm_name, term.name AS term_name`+teacherOfferingJoins+`
		JOIN subjects subject ON subject.tenant_id=o.tenant_id AND subject.id=o.subject_id AND subject.status='ACTIVE'
		JOIN classes c ON c.tenant_id=o.tenant_id AND c.id=o.class_id AND c.status='ACTIVE'
		JOIN academic_terms term ON term.tenant_id=o.tenant_id AND term.id=o.term_id AND term.session_id=c.session_id
		JOIN academic_sessions session ON session.tenant_id=o.tenant_id AND session.id=term.session_id AND session.status<>'ARCHIVED'
		LEFT JOIN class_arms arm ON arm.tenant_id=c.tenant_id AND arm.class_id=c.id AND arm.id=o.class_arm_id
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND (o.class_arm_id IS NULL OR arm.status='ACTIVE')
		AND ($3::uuid IS NULL OR o.id>$3) ORDER BY o.id LIMIT 51`)
}

func (h *Handler) ListTeachingObjectives(w http.ResponseWriter, r *http.Request) {
	offering, ok := teachingRouteID(w, r, "offeringID")
	if !ok {
		return
	}
	teacherPage(h, w, r, pgx.RowToStructByName[teachingObjective], func(o teachingObjective) uuid.UUID { return o.ID },
		`SELECT p.id,p.code,p.objective_text,`+teachingEnrichment("term.session_id", "ARRAY[]::uuid[]")+teacherOfferingJoins+`
		JOIN academic_terms term ON term.tenant_id=o.tenant_id AND term.id=o.term_id
		JOIN academic_sessions session ON session.tenant_id=o.tenant_id AND session.id=term.session_id AND session.status<>'ARCHIVED'
		JOIN classes c ON c.tenant_id=o.tenant_id AND c.id=o.class_id AND c.session_id=term.session_id AND c.status='ACTIVE'
		JOIN grade_levels g ON g.tenant_id=c.tenant_id AND g.id=c.grade_level_id AND g.status='ACTIVE'
		JOIN school_current_curriculum_selections s ON s.tenant_id=o.tenant_id AND s.session_id=term.session_id
			AND s.stage=g.stage AND s.curriculum_role='PRIMARY'
		JOIN curriculum_framework_versions f ON f.id=s.framework_version_id AND f.status='PUBLISHED'
		JOIN curriculum_objectives p ON p.framework_version_id=f.id AND p.stage=s.stage
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND o.id=$4
		AND ($3::uuid IS NULL OR p.id>$3) ORDER BY p.id LIMIT 51`, offering)
}

const ownedLessonJoin = teacherOfferingJoins + `
	JOIN lessons l ON l.tenant_id=o.tenant_id AND l.offering_id=o.id AND l.author_membership_id=m.id
	JOIN curriculum_objectives p ON p.id=l.primary_objective_id
 JOIN school_curriculum_selections pinned ON pinned.tenant_id=l.tenant_id AND pinned.id=l.curriculum_selection_id
	LEFT JOIN lesson_reviews review ON review.tenant_id=l.tenant_id AND review.lesson_id=l.id AND review.lesson_revision=l.content_revision
	WHERE t.slug=$1::citext AND t.status='ACTIVE'
`

func (h *Handler) ListTeacherLessons(w http.ResponseWriter, r *http.Request) {
	teacherPage(h, w, r, pgx.RowToStructByName[teacherLessonRecord], func(l teacherLessonRecord) uuid.UUID { return l.ID },
		`SELECT l.id,l.offering_id,l.title,l.status,NULL::text AS content,p.code AS objective_code,
		COALESCE(t.feature_flags->'lesson_review_required'='true'::jsonb,false) AS review_required,
		CASE WHEN review.approved_at IS NOT NULL THEN 'APPROVED'
			WHEN review.lesson_id IS NOT NULL THEN 'PENDING' ELSE 'UNSUBMITTED' END AS review_status,NULL::jsonb AS enrichment,false AS media_upload_enabled,false AS file_submission_enabled`+ownedLessonJoin+
			`AND ($3::uuid IS NULL OR l.id>$3) ORDER BY l.id LIMIT 51`)
}

func (h *Handler) GetTeacherLesson(w http.ResponseWriter, r *http.Request) {
	id, ok := teachingRouteID(w, r, "lessonID")
	if !ok {
		return
	}
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT l.id,l.offering_id,l.title,l.status,l.content,p.code AS objective_code,
		COALESCE(t.feature_flags->'lesson_review_required'='true'::jsonb,false) AS review_required,
		CASE WHEN review.approved_at IS NOT NULL THEN 'APPROVED'
			WHEN review.lesson_id IS NOT NULL THEN 'PENDING' ELSE 'UNSUBMITTED' END AS review_status,`+teachingEnrichment("pinned.session_id", "l.enrichment_alignment_ids")+`,($4::boolean AND l.status='DRAFT') AS media_upload_enabled,$4::boolean AS file_submission_enabled`+ownedLessonJoin+`AND l.id=$3`,
		chi.URLParam(r, "slug"), actor, id, h.mediaStore != nil)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	item, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[teacherLessonRecord])
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func teachingRouteID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil || id == uuid.Nil {
		w.Header().Set("Cache-Control", "no-store")
		lessonUnavailable(w, r)
		return uuid.Nil, false
	}
	return id, true
}

func teachingActor(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	w.Header().Set("Cache-Control", "no-store")
	if actor := auth.GetUser(r.Context()); actor != nil && actor.ID != uuid.Nil {
		return actor.ID, true
	}
	response.ErrorJSON(w, r, http.StatusUnauthorized, response.ErrCodeUnauthenticated, "Authentication required", nil)
	return uuid.Nil, false
}

func teacherPage[T any](h *Handler, w http.ResponseWriter, r *http.Request, scan pgx.RowToFunc[T], identify func(T) uuid.UUID, query string, extra ...any) {
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	var after *uuid.UUID
	if raw := r.URL.Query().Get("after"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid teaching cursor", nil)
			return
		}
		after = &id
	}
	rows, err := h.pool.Query(r.Context(), query, append([]any{chi.URLParam(r, "slug"), actor, after}, extra...)...)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, scan)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	if items == nil {
		items = []T{}
	}
	var next *uuid.UUID
	if len(items) > 50 {
		id := identify(items[49])
		next = &id
		items = items[:50]
	}
	response.JSON(w, r, http.StatusOK, struct {
		Items []T        `json:"items"`
		Next  *uuid.UUID `json:"next,omitempty"`
	}{items, next})
}
