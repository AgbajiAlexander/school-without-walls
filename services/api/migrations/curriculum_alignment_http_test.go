package migrations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/db"
	apphttp "github.com/school-without-walls/api/internal/platform/http"
)

func testCurriculumAlignmentHTTP(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	write := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	school, principal, operator := uuid.New(), uuid.New(), uuid.New()
	write(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'alignment-http','Fictional School','ACTIVE')`, school)
	write(`INSERT INTO users (id,email,password_hash,full_name,status) VALUES
		($1,'review-principal@example.test','unused','Fictional Principal','ACTIVE'),
		($2,'review-operator@example.test','unused','Fictional Operator','ACTIVE')`, principal, operator)
	write(`INSERT INTO memberships (id,tenant_id,user_id,role) VALUES ($1,$2,$3,'PRINCIPAL')`, uuid.New(), school, principal)
	write(`INSERT INTO platform_admins (user_id) VALUES ($1)`, operator)
	principalToken, operatorToken, csrf := uuid.NewString(), uuid.NewString(), uuid.NewString()
	write(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at) VALUES
		($1,$2,$5,'127.0.0.1','Fixture',clock_timestamp() + interval '1 hour'),
		($3,$4,$5,'127.0.0.1','Fixture',clock_timestamp() + interval '1 hour')`,
		principal, auth.HashToken(principalToken), operator, auth.HashToken(operatorToken), auth.HashToken(csrf))

	primaryEdition, relatedEdition, primary, related := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	write(`INSERT INTO curriculum_framework_versions
		(id,jurisdiction,publisher,title,edition,framework_kind,effective_from,source_url,license_note) VALUES
		($1,'NG','HTTP Fixture','Fictional Nigerian','one','NIGERIAN_PRIMARY','2030-01-01','https://example.test/primary','Fixture only'),
		($2,'GB-ENG','HTTP Fixture','Fictional Enrichment','one','ENRICHMENT','2030-01-01','https://example.test/enrichment','Fixture only')`,
		primaryEdition, relatedEdition)
	write(`INSERT INTO curriculum_objectives (id,framework_version_id,stage,grade_band,subject,strand,code,objective_text) VALUES
		($1,$2,'PRIMARY','Primary 1','Mathematics','Number','FIX.1','Fictional Nigerian objective'),
		($3,$4,'PRIMARY','Year 1','Mathematics','Number','FIX.1','Fictional enrichment objective')`,
		primary, primaryEdition, related, relatedEdition)
	router, err := apphttp.NewRouter(apphttp.RouterConfig{Config: &config.Config{Environment: "test"}, DB: &db.DB{Pool: pool}})
	if err != nil {
		t.Fatal(err)
	}
	send := func(method, path, token, header, body string, want int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		req.Header.Set(auth.HeaderCSRF, header)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("alignment HTTP status %d, want %d", rec.Code, want)
		}
		return rec.Body.Bytes()
	}

	const alignments = "/api/v1/curriculum/alignments"
	payload := `{"primary_objective_id":"` + primary.String() + `","related_objective_id":"` + related.String() + `","relation":"COMPLEMENTARY"}`
	send("POST", alignments, "", "", payload, 401)
	send("POST", alignments, principalToken, csrf, payload, 403)
	send("POST", alignments, operatorToken, "", payload, 403)
	send("POST", alignments, operatorToken, csrf, strings.TrimSuffix(payload, "}")+`,"review_status":"HUMAN_REVIEWED"}`, 422)
	var result struct{ Data struct{ ID uuid.UUID } }
	if err := json.Unmarshal(send("POST", alignments, operatorToken, csrf, payload, 201), &result); err != nil || result.Data.ID == uuid.Nil {
		t.Fatal("draft response omitted alignment ID")
	}
	send("POST", alignments, operatorToken, csrf, payload, 409)
	list := alignments + "?primary_objective_id=" + primary.String()
	assertVisible := func(want int) {
		t.Helper()
		var result struct {
			Data struct {
				Items []struct {
					RelatedObjectiveID uuid.UUID `json:"related_objective_id"`
					ReviewedAt         string    `json:"reviewed_at"`
				}
			}
		}
		body := send("GET", list, principalToken, "", "", 200)
		if err := json.Unmarshal(body, &result); err != nil || len(result.Data.Items) != want {
			t.Fatal("unexpected verified alignment visibility")
		}
		if want != 0 && (result.Data.Items[0].RelatedObjectiveID != related || result.Data.Items[0].ReviewedAt == "" ||
			strings.Contains(string(body), operator.String())) {
			t.Fatal("verified response has incorrect mapping or exposed reviewer identity")
		}
	}
	assertVisible(0)
	review := alignments + "/" + result.Data.ID.String() + "/review"
	send("POST", review, principalToken, csrf, "{}", 403)
	send("POST", review, operatorToken, "", "{}", 403)
	send("POST", review, operatorToken, csrf, `{"reviewer_id":"`+principal.String()+`"}`, 422)
	send("POST", review, operatorToken, csrf, "{}", 409)
	write(`UPDATE curriculum_framework_versions SET status='PUBLISHED' WHERE id IN ($1,$2)`, primaryEdition, relatedEdition)
	send("POST", review, operatorToken, csrf, "{}", 200)
	send("POST", review, operatorToken, csrf, "{}", 409)
	assertVisible(1)
	write(`UPDATE curriculum_framework_versions SET status='WITHDRAWN' WHERE id=$1`, relatedEdition)
	assertVisible(0)
	write(`DELETE FROM platform_admins WHERE user_id=$1`, operator)
	send("POST", alignments, operatorToken, csrf, payload, 403)

	var drafts, reviews, otherActors int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE action='curriculum.alignment.create'),
		count(*) FILTER (WHERE action='curriculum.alignment.review'), count(*) FILTER (WHERE actor_id IS DISTINCT FROM $1)
		FROM audit_events WHERE subject_type='curriculum_alignment' AND subject_id=$2 AND tenant_id IS NULL`,
		operator, result.Data.ID.String()).Scan(&drafts, &reviews, &otherActors); err != nil || drafts != 1 || reviews != 1 || otherActors != 0 {
		t.Fatal("expected exactly one global draft audit and one review audit for the authenticated actor")
	}
}
