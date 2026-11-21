package learning

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/school-without-walls/api/internal/platform/response"
)

const progressState = `CASE WHEN progress.completed_at IS NOT NULL THEN 'COMPLETED'
	WHEN progress.started_at IS NOT NULL THEN 'IN_PROGRESS' ELSE 'NOT_STARTED' END`

func (h *Handler) SetLessonProgress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := teachingActor(w, r)
	if !ok {
		return
	}
	id, ok := teachingRouteID(w, r, "lessonID")
	if !ok {
		return
	}
	var input struct {
		State string `json:"state"`
	}
	if !decodeLesson(w, r, &input) || (input.State != "IN_PROGRESS" && input.State != "COMPLETED") {
		invalidLesson(w, r)
		return
	}
	var progress struct {
		State       string     `json:"state"`
		StartedAt   time.Time  `json:"started_at"`
		CompletedAt *time.Time `json:"completed_at,omitempty"`
	}
	// The statement derives the target student from the same enrollment
	// predicate as lesson reads. The unique key serializes concurrent updates.
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO lesson_progress AS progress (tenant_id,lesson_id,student_id,started_at,completed_at)
		SELECT t.id,l.id,s.id,statement_timestamp(),
			CASE WHEN $5::boolean THEN statement_timestamp() END`+studentLessonScope+` LIMIT 1
		ON CONFLICT (tenant_id,lesson_id,student_id) DO UPDATE
		SET completed_at=COALESCE(progress.completed_at,CASE WHEN EXCLUDED.completed_at IS NOT NULL THEN GREATEST(EXCLUDED.completed_at,progress.started_at) END)
		RETURNING `+progressState+`,progress.started_at,progress.completed_at`,
		chi.URLParam(r, "slug"), actor, id, nil, input.State == "COMPLETED").Scan(&progress.State, &progress.StartedAt, &progress.CompletedAt)
	if err != nil {
		lessonReadError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, progress)
}
