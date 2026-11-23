package academics

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

type AssessmentHandler struct{ pool *pgxpool.Pool }

func NewAssessmentHandler(pool *pgxpool.Pool) *AssessmentHandler {
	return &AssessmentHandler{pool: pool}
}

type schemeComponent struct {
	Name     string `json:"name"`
	MaxScore string `json:"max_score"`
	Weight   string `json:"weight"`
}

type gradeBand struct {
	Label string `json:"label"`
	MinBP *int   `json:"min_bp"`
	MaxBP *int   `json:"max_bp"`
}

type schemeInput struct {
	GradeBands *[]gradeBand      `json:"grade_bands"`
	Stage      string            `json:"stage"`
	Revision   *int64            `json:"revision"`
	Components []schemeComponent `json:"components"`
}

type schemeResponse struct {
	GradeBands []gradeBand       `json:"grade_bands"`
	ID         uuid.UUID         `json:"id"`
	Stage      string            `json:"stage"`
	Version    int               `json:"version"`
	Status     string            `json:"status"`
	Revision   int64             `json:"revision"`
	Components []schemeComponent `json:"components"`
}

func (h *AssessmentHandler) List(w http.ResponseWriter, r *http.Request)   { h.handle(w, r, "list") }
func (h *AssessmentHandler) Create(w http.ResponseWriter, r *http.Request) { h.handle(w, r, "create") }
func (h *AssessmentHandler) Edit(w http.ResponseWriter, r *http.Request)   { h.handle(w, r, "edit") }
func (h *AssessmentHandler) Publish(w http.ResponseWriter, r *http.Request) {
	h.handle(w, r, "publish")
}

func (h *AssessmentHandler) handle(w http.ResponseWriter, r *http.Request, action string) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := academicActor(w, r)
	if !ok {
		return
	}
	term, err := uuid.Parse(chi.URLParam(r, "termID"))
	if err != nil || term == uuid.Nil {
		(&Handler{}).writeError(w, r, ErrNotFound)
		return
	}
	var id uuid.UUID
	if action == "edit" || action == "publish" {
		id, err = uuid.Parse(chi.URLParam(r, "schemeID"))
		if err != nil || id == uuid.Nil {
			(&Handler{}).writeError(w, r, ErrNotFound)
			return
		}
	}
	var input schemeInput
	if action != "list" {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || !errors.Is(decoder.Decode(new(any)), io.EOF) || !validateScheme(&input, action) {
			response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Check the scheme components and their values", nil)
			return
		}
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	tenant, err := resolveTenant(ctx, tx, chi.URLParam(r, "slug"), actor.ID, true, action != "list")
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	if action == "list" {
		listAssessmentSchemes(w, r, tx, tenant, term)
		return
	}
	var locked uuid.UUID
	err = tx.QueryRow(ctx, `SELECT sess.id FROM academic_sessions sess JOIN academic_terms term ON term.tenant_id=sess.tenant_id AND term.session_id=sess.id
  WHERE sess.tenant_id=$1 AND term.id=$2 AND sess.status<>'ARCHIVED' FOR SHARE OF sess`, tenant, term).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	err = tx.QueryRow(ctx, `SELECT id FROM academic_terms WHERE tenant_id=$1 AND id=$2 AND session_id=$3 FOR UPDATE`, tenant, term, locked).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}

	if action == "create" {
		id = uuid.New()
		tag, insertErr := tx.Exec(ctx, `INSERT INTO assessment_schemes (id,tenant_id,term_id,stage,version)
   SELECT $1,$2,$3,$4,COALESCE(max(version),0)+1 FROM assessment_schemes
   WHERE tenant_id=$2 AND term_id=$3 AND stage=$4
   HAVING count(*) FILTER (WHERE status='DRAFT')=0`, id, tenant, term, input.Stage)
		err = insertErr
		if err == nil && tag.RowsAffected() != 1 {
			schemeConflict(w, r)
			return
		}
	} else {
		var revision int64
		var status string
		err = tx.QueryRow(ctx, `SELECT status,components_revision FROM assessment_schemes WHERE tenant_id=$1 AND term_id=$2 AND id=$3 FOR UPDATE`,
			tenant, term, id).Scan(&status, &revision)
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
		if err == nil && (status != "DRAFT" || revision != *input.Revision) {
			schemeConflict(w, r)
			return
		}
	}
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	if action == "publish" {
		_, err = tx.Exec(ctx, `UPDATE assessment_schemes SET status='PUBLISHED' WHERE tenant_id=$1 AND id=$2`, tenant, id)
	} else {
		if action == "edit" {
			_, err = tx.Exec(ctx, `DELETE FROM assessment_components WHERE tenant_id=$1 AND scheme_id=$2`, tenant, id)
		}
		for _, part := range input.Components {
			if err != nil {
				break
			}
			_, err = tx.Exec(ctx, `INSERT INTO assessment_components (id,tenant_id,scheme_id,name,max_score,weight)
    VALUES ($1,$2,$3,$4,$5::text::numeric,$6::text::numeric)`, uuid.New(), tenant, id, part.Name, part.MaxScore, part.Weight)
		}
	}
	if err == nil && input.GradeBands != nil {
		var encoded []byte
		encoded, err = json.Marshal(input.GradeBands)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE assessment_schemes SET grade_bands=$3::jsonb, components_revision=components_revision+1
				WHERE tenant_id=$1 AND id=$2`, tenant, id, encoded)
		}
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && (databaseError.Code == "23514" || databaseError.Code == "23505") {
		response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Check component names, publication weights and non-overlapping grade bands", nil)
		return
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,tenant_id,actor_id,action,subject_type,subject_id,request_id)
    VALUES ($1,$2,$3,$4,'assessment_scheme',$5,$6)`, uuid.New(), tenant, actor.ID, "assessment.scheme."+action, id.String(), middleware.GetRequestID(ctx))
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	code := http.StatusOK
	if action == "create" {
		code = http.StatusCreated
	}
	response.JSON(w, r, code, map[string]uuid.UUID{"id": id})
}

func listAssessmentSchemes(w http.ResponseWriter, r *http.Request, tx pgx.Tx, tenant, term uuid.UUID) {
	var title string
	var archived bool
	err := tx.QueryRow(r.Context(), `SELECT term.name,sess.status='ARCHIVED' FROM academic_terms term
  JOIN academic_sessions sess ON sess.tenant_id=term.tenant_id AND sess.id=term.session_id
  WHERE term.tenant_id=$1 AND term.id=$2`, tenant, term).Scan(&title, &archived)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT s.id,s.stage,s.version,s.status,s.components_revision AS revision,s.grade_bands,ARRAY_TO_JSON(ARRAY(
  SELECT json_build_object('name',c.name::text,'max_score',c.max_score::text,'weight',c.weight::text)
  FROM assessment_components c WHERE c.tenant_id=s.tenant_id AND c.scheme_id=s.id ORDER BY c.name,c.id
 )) AS components FROM assessment_schemes s WHERE s.tenant_id=$1 AND s.term_id=$2
 ORDER BY s.stage,s.version DESC LIMIT 100`, tenant, term)
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	schemes, err := pgx.CollectRows(rows, pgx.RowToStructByName[schemeResponse])
	if err != nil {
		(&Handler{}).writeError(w, r, err)
		return
	}
	if schemes == nil {
		schemes = []schemeResponse{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"term": title, "archived": archived, "items": schemes})
}

var schemeDecimal = regexp.MustCompile(`^[0-9]{1,7}(\.[0-9]{1,2})?$`)

func validateScheme(input *schemeInput, action string) bool {
	if action == "publish" {
		return input.Revision != nil && *input.Revision >= 0 && input.Stage == "" && input.Components == nil && input.GradeBands == nil
	}
	if action == "create" {
		if input.Revision != nil || (input.Stage != "PRIMARY" && input.Stage != "JUNIOR_SECONDARY" && input.Stage != "SENIOR_SECONDARY") {
			return false
		}
	} else if input.Revision == nil || *input.Revision < 0 || input.Stage != "" {
		return false
	}
	if input.GradeBands != nil {
		if len(*input.GradeBands) > 30 {
			return false
		}
		for i := range *input.GradeBands {
			band := &(*input.GradeBands)[i]
			band.Label = strings.TrimSpace(band.Label)
			if band.Label == "" || utf8.RuneCountInString(band.Label) > 40 || strings.ContainsRune(band.Label, 0) ||
				band.MinBP == nil || band.MaxBP == nil || *band.MinBP < 0 || *band.MaxBP > 10000 || *band.MinBP > *band.MaxBP {
				return false
			}
		}
	}
	if len(input.Components) < 1 || len(input.Components) > 30 {
		return false
	}
	for i := range input.Components {
		part := &input.Components[i]
		part.Name = strings.TrimSpace(part.Name)
		if part.Name == "" || utf8.RuneCountInString(part.Name) > 100 || strings.ContainsRune(part.Name, 0) ||
			!schemeDecimal.MatchString(part.Weight) || !schemeDecimal.MatchString(part.MaxScore) {
			return false
		}
		weight, _ := strconv.ParseFloat(part.Weight, 64)
		maximum, _ := strconv.ParseFloat(part.MaxScore, 64)
		if weight <= 0 || weight > 100 || maximum <= 0 {
			return false
		}
	}
	return true
}

func schemeConflict(w http.ResponseWriter, r *http.Request) {
	response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "The scheme changed or a draft already exists. Refresh to continue.", nil)
}
