package learning

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

const scoreOffering = teacherOfferingJoins + `
	JOIN academic_terms term ON term.tenant_id=o.tenant_id AND term.id=o.term_id
	JOIN academic_sessions sess ON sess.tenant_id=term.tenant_id AND sess.id=term.session_id AND sess.status='ACTIVE'
	JOIN classes c ON c.tenant_id=o.tenant_id AND c.id=o.class_id AND c.status='ACTIVE'
	JOIN grade_levels g ON g.tenant_id=c.tenant_id AND g.id=c.grade_level_id AND g.status='ACTIVE'
	JOIN assessment_schemes scheme ON scheme.tenant_id=o.tenant_id AND scheme.term_id=term.id
		AND scheme.stage=g.stage AND scheme.status='PUBLISHED'
	JOIN assessment_components component ON component.tenant_id=scheme.tenant_id AND component.scheme_id=scheme.id
`

const scorePlacements = `
	FROM enrollments placement
	JOIN students student ON student.tenant_id=placement.tenant_id AND student.id=placement.student_id
		AND student.status='ACTIVE'
	LEFT JOIN class_arms arm ON arm.tenant_id=placement.tenant_id AND arm.id=placement.class_arm_id
	WHERE placement.tenant_id=o.tenant_id AND placement.term_id=o.term_id AND placement.class_id=o.class_id
		AND placement.status='ACTIVE' AND (o.class_arm_id IS NULL OR o.class_arm_id=placement.class_arm_id)
		AND (placement.class_arm_id IS NULL OR arm.status='ACTIVE')
`

type scoreRosterItem struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Maximum     int32     `json:"maximum_hundredths"`
	Score       *int32    `json:"score_hundredths"`
	Revision    int64     `json:"revision"`
}

func (h *Handler) ListAssessmentScores(w http.ResponseWriter, r *http.Request) {
	offering, ok := teachingRouteID(w, r, "offeringID")
	if !ok {
		return
	}
	component, ok := teachingRouteID(w, r, "componentID")
	if !ok {
		return
	}
	teacherPage(h, w, r, pgx.RowToStructByName[scoreRosterItem], func(row scoreRosterItem) uuid.UUID { return row.ID },
		`SELECT roster.id,roster.display_name,(component.max_score*100)::integer AS maximum,
			(mark.score*100)::integer AS score,COALESCE(mark.revision,0) AS revision`+scoreOffering+`
		JOIN LATERAL (SELECT placement.id,student.display_name`+scorePlacements+`
			AND ($3::uuid IS NULL OR placement.id>$3)) roster ON true
		LEFT JOIN assessment_scores mark ON mark.tenant_id=o.tenant_id AND mark.offering_id=o.id
			AND mark.enrollment_id=roster.id AND mark.component_id=component.id
		WHERE t.slug=$1::citext AND t.status='ACTIVE' AND o.id=$4 AND component.id=$5
		ORDER BY roster.id LIMIT 51`, offering, component)
}

func (h *Handler) SaveAssessmentScore(w http.ResponseWriter, r *http.Request) {
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	offering, ok := teachingRouteID(w, r, "offeringID")
	if !ok {
		return
	}
	component, ok := teachingRouteID(w, r, "componentID")
	if !ok {
		return
	}
	placement, ok := teachingRouteID(w, r, "placementID")
	if !ok {
		return
	}
	var input struct {
		Score    *int32 `json:"score_hundredths"`
		Revision *int64 `json:"revision"`
	}
	if !decodeLesson(w, r, &input) || input.Score == nil || *input.Score < 0 || input.Revision == nil || *input.Revision < 0 {
		invalidAssessmentScore(w, r)
		return
	}
	ctx := r.Context()
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)

	const scope = ` WHERE t.slug=$1::citext AND t.status='ACTIVE' AND o.id=$3 AND component.id=$4`
	args := []any{chi.URLParam(r, "slug"), actor, offering, component, placement}
	var tenant, student uuid.UUID
	var maximum int32
	err = tx.QueryRow(ctx, `SELECT t.id,roster.student_id,(component.max_score*100)::integer`+scoreOffering+`
		JOIN LATERAL (SELECT student.id AS student_id`+scorePlacements+` AND placement.id=$5) roster ON true`+scope+
		` FOR SHARE OF t,u,m,a,o,term,sess,c,g`, args...).Scan(&tenant, &student, &maximum)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}

	// Enrollment mutations and score writers serialize on the learner before touching a placement.
	err = tx.QueryRow(ctx, `SELECT id FROM students WHERE tenant_id=$1 AND id=$2 AND status='ACTIVE' FOR UPDATE`,
		tenant, student).Scan(&student)
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT eligible.id`+scoreOffering+`
			JOIN LATERAL (SELECT placement.id`+scorePlacements+` AND placement.id=$5) eligible ON true`+scope,
			args...).Scan(&placement)
	}
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	if *input.Score > maximum {
		invalidAssessmentScore(w, r)
		return
	}

	var id uuid.UUID
	var previous *int32
	var revision int64
	err = tx.QueryRow(ctx, `SELECT id,(score*100)::integer,revision FROM assessment_scores
		WHERE tenant_id=$1 AND enrollment_id=$2 AND offering_id=$3 AND component_id=$4`,
		tenant, placement, offering, component).Scan(&id, &previous, &revision)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		lessonReadError(w, r, err)
		return
	}
	if revision != *input.Revision {
		response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "The score changed. Refresh and try again.", nil)
		return
	}
	if previous == nil {
		id = uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO assessment_scores
			(id,tenant_id,enrollment_id,offering_id,component_id,score)
			VALUES ($1,$2,$3,$4,$5,($6::integer/100.00)::numeric(9,2))`, id, tenant, placement, offering, component, *input.Score)
	} else {
		_, err = tx.Exec(ctx, `UPDATE assessment_scores SET score=($3::integer/100.00)::numeric(9,2),revision=revision+1
			WHERE tenant_id=$1 AND id=$2`, tenant, id, *input.Score)
	}
	if err == nil {
		revision++
		_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id,metadata_redacted)
			VALUES ($1,$2,$3,'assessment.score.saved','assessment_score',$4,$5,
			jsonb_build_object('previous_hundredths',$6::integer,'score_hundredths',$7::integer,'revision',$8::bigint))`,
			uuid.New(), tenant, actor, id.String(), middleware.GetRequestID(ctx), previous, *input.Score, revision)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, struct {
		Score    int32 `json:"score_hundredths"`
		Revision int64 `json:"revision"`
	}{*input.Score, revision})
}

func invalidAssessmentScore(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Enter a score within the component range and a valid revision", nil)
}
