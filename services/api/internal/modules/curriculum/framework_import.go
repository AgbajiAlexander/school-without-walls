package curriculum

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/middleware"
	"github.com/school-without-walls/api/internal/platform/response"
)

var jurisdictionPattern = regexp.MustCompile(`^[A-Z]{2}(-[A-Z0-9]{1,8})?$`)
var objectiveCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{0,63}$`)

type importObjective struct {
	Stage     string `json:"stage"`
	GradeBand string `json:"grade_band"`
	Subject   string `json:"subject"`
	Strand    string `json:"strand"`
	Code      string `json:"code"`
	Text      string `json:"objective_text"`
}

type frameworkImport struct {
	Jurisdiction    string            `json:"jurisdiction"`
	Publisher       string            `json:"publisher"`
	Title           string            `json:"title"`
	Edition         string            `json:"edition"`
	Kind            string            `json:"framework_kind"`
	EffectiveFrom   string            `json:"effective_from"`
	EffectiveTo     *string           `json:"effective_to"`
	SourceURL       string            `json:"source_url"`
	LicenseNote     string            `json:"license_note"`
	RightsConfirmed bool              `json:"rights_confirmed"`
	Objectives      []importObjective `json:"objectives"`
}

func (input *frameworkImport) validate() bool {
	input.Jurisdiction = strings.TrimSpace(input.Jurisdiction)
	input.Publisher = strings.TrimSpace(input.Publisher)
	input.Title = strings.TrimSpace(input.Title)
	input.Edition = strings.TrimSpace(input.Edition)
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.LicenseNote = strings.TrimSpace(input.LicenseNote)
	if !input.RightsConfirmed || len(input.Objectives) == 0 || len(input.Objectives) > 50 ||
		!jurisdictionPattern.MatchString(input.Jurisdiction) ||
		!boundedText(input.Publisher, 150) || !boundedText(input.Title, 200) ||
		!boundedText(input.Edition, 80) || !boundedText(input.LicenseNote, 2000) ||
		!boundedText(input.SourceURL, 2048) {
		return false
	}
	switch input.Kind {
	case "NIGERIAN_PRIMARY":
		if input.Jurisdiction != "NG" {
			return false
		}
	case "DEVELOPMENTAL", "ENRICHMENT":
	default:
		return false
	}
	source, err := url.Parse(input.SourceURL)
	if err != nil || source.Scheme != "https" || source.Hostname() == "" || source.User != nil ||
		source.RawQuery != "" || source.Fragment != "" || source.ForceQuery ||
		strings.IndexFunc(input.SourceURL, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return false
	}
	start, err := time.Parse(time.DateOnly, input.EffectiveFrom)
	if err != nil || start.Year() < 1 {
		return false
	}
	if input.EffectiveTo != nil {
		end, err := time.Parse(time.DateOnly, *input.EffectiveTo)
		if err != nil || end.Before(start) {
			return false
		}
	}
	codes := make(map[string]bool, len(input.Objectives))
	for i := range input.Objectives {
		item := &input.Objectives[i]
		item.Code = strings.TrimSpace(item.Code)
		item.GradeBand = strings.TrimSpace(item.GradeBand)
		item.Subject = strings.TrimSpace(item.Subject)
		item.Strand = strings.TrimSpace(item.Strand)
		item.Text = strings.TrimSpace(item.Text)
		if !validCurriculumStage(item.Stage) || !objectiveCodePattern.MatchString(item.Code) || codes[item.Code] ||
			!boundedText(item.GradeBand, 60) || !boundedText(item.Subject, 100) ||
			!boundedText(item.Strand, 100) || !boundedText(item.Text, 2000) {
			return false
		}
		codes[item.Code] = true
	}
	return true
}

func boundedText(value string, limit int) bool {
	return value != "" && utf8.RuneCountInString(value) <= limit && !strings.ContainsRune(value, 0)
}

func (h *Handler) ImportFramework(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor := auth.GetUser(r.Context())
	if actor == nil || actor.ID == uuid.Nil {
		response.ErrorJSON(w, r, http.StatusUnauthorized, response.ErrCodeUnauthenticated, "Authentication required", nil)
		return
	}
	var input frameworkImport
	if !decodeCurriculumRequest(w, r, &input, 256<<10) || !input.validate() {
		response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Invalid curriculum import or missing rights confirmation", nil)
		return
	}
	id := uuid.New()
	ctx := r.Context()
	err := h.writeCatalogChange(ctx, actor.ID, id, "curriculum.framework.import", "curriculum_framework",
		middleware.GetRequestID(ctx), func(ctx context.Context, tx pgx.Tx, _ uuid.UUID) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO curriculum_framework_versions
					(id,jurisdiction,publisher,title,edition,framework_kind,effective_from,effective_to,source_url,license_note)
				VALUES ($1,$2,$3,$4,$5,$6,$7::text::date,$8::text::date,$9,$10)
			`, id, input.Jurisdiction, input.Publisher, input.Title, input.Edition, input.Kind,
				input.EffectiveFrom, input.EffectiveTo, input.SourceURL, input.LicenseNote)
			if err != nil {
				return err
			}
			for _, item := range input.Objectives {
				if _, err := tx.Exec(ctx, `
					INSERT INTO curriculum_objectives
						(id,framework_version_id,stage,grade_band,subject,strand,code,objective_text)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
				`, uuid.New(), id, item.Stage, item.GradeBand, item.Subject, item.Strand, item.Code, item.Text); err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		writeCatalogError(w, r, err, "Unable to import curriculum framework")
		return
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{"id": id, "status": "DRAFT", "objective_count": len(input.Objectives)})
}

func (h *Handler) PublishFramework(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor := auth.GetUser(r.Context())
	if actor == nil || actor.ID == uuid.Nil {
		response.ErrorJSON(w, r, http.StatusUnauthorized, response.ErrCodeUnauthenticated, "Authentication required", nil)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "frameworkID"))
	var review struct {
		RightsReviewed  bool `json:"rights_reviewed"`
		ContentReviewed bool `json:"content_reviewed"`
	}
	if err != nil || id == uuid.Nil || !decodeCurriculumRequest(w, r, &review, 4096) ||
		!review.RightsReviewed || !review.ContentReviewed {
		response.ErrorJSON(w, r, http.StatusUnprocessableEntity, response.ErrCodeInvalidInput, "Rights and content review confirmations are required", nil)
		return
	}
	ctx := r.Context()
	err = h.writeCatalogChange(ctx, actor.ID, id, "curriculum.framework.publish", "curriculum_framework",
		middleware.GetRequestID(ctx), func(ctx context.Context, tx pgx.Tx, _ uuid.UUID) error {
			var published uuid.UUID
			return tx.QueryRow(ctx, `
				UPDATE curriculum_framework_versions f SET status='PUBLISHED'
				WHERE f.id=$1 AND f.status='DRAFT' AND EXISTS (
					SELECT 1 FROM curriculum_objectives o WHERE o.framework_version_id=f.id
				) RETURNING f.id
			`, id).Scan(&published)
		})
	if errors.Is(err, pgx.ErrNoRows) {
		response.ErrorJSON(w, r, http.StatusConflict, response.ErrCodeConflict, "No draft edition with objectives is available to publish", nil)
		return
	}
	if err != nil {
		writeCatalogError(w, r, err, "Unable to publish curriculum framework")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"id": id, "status": "PUBLISHED"})
}

func (h *Handler) PreviewImportedFramework(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, err := uuid.Parse(chi.URLParam(r, "frameworkID"))
	if err != nil || id == uuid.Nil {
		invalidQuery(w, r)
		return
	}
	type previewObjective struct {
		ID            uuid.UUID `json:"id"`
		Stage         string    `json:"stage"`
		GradeBand     string    `json:"grade_band"`
		Subject       string    `json:"subject"`
		Strand        string    `json:"strand"`
		Code          string    `json:"code"`
		ObjectiveText string    `json:"objective_text"`
		SourceURL     string    `json:"source_url"`
		LicenseNote   string    `json:"license_note"`
	}
	browse(h, w, r, reviewerAccess, pgx.RowToStructByName[previewObjective], func(item previewObjective) uuid.UUID { return item.ID },
		`SELECT o.id, o.stage, o.grade_band, o.subject, o.strand, o.code, o.objective_text, f.source_url, f.license_note
		FROM curriculum_objectives o JOIN curriculum_framework_versions f ON f.id=o.framework_version_id
		WHERE f.id=$3 AND f.status='DRAFT' AND ($1::uuid IS NULL OR o.id>$1)
		ORDER BY o.id LIMIT $2`, []any{id})
}
