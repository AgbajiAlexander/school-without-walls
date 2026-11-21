package migrations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/db"
	apphttp "github.com/school-without-walls/api/internal/platform/http"
)

func testCurriculumCatalogHTTP(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	write := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	school, member, operator := uuid.New(), uuid.New(), uuid.New()
	write(`INSERT INTO tenants (id,slug,name,status) VALUES ($1,'catalog-fixture','Fictional School','ACTIVE')`, school)
	write(`INSERT INTO users (id,email,password_hash,full_name,status) VALUES
		($1,'catalog-member@example.test','unused','Fictional Member','ACTIVE'),
		($2,'catalog-operator@example.test','unused','Fictional Operator','ACTIVE')`, member, operator)
	write(`INSERT INTO platform_admins (user_id) VALUES ($1)`, operator)
	write(`INSERT INTO memberships (id,tenant_id,user_id,role) VALUES ($1,$2,$3,'TEACHER')`, uuid.New(), school, member)
	memberToken, operatorToken := uuid.NewString(), uuid.NewString()
	write(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at) VALUES
		($1,$2,$5,'127.0.0.1','Fixture',clock_timestamp() + interval '1 hour'),
		($3,$4,$5,'127.0.0.1','Fixture',clock_timestamp() + interval '1 hour')`,
		member, auth.HashToken(memberToken), operator, auth.HashToken(operatorToken), auth.HashToken(uuid.NewString()))

	nigeria, enrichment, draft, withdrawn := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	write(`INSERT INTO curriculum_framework_versions
		(id,jurisdiction,publisher,title,edition,framework_kind,effective_from,source_url,license_note) VALUES
		($1,'NG','Catalog Fixture','Fictional Framework','one','NIGERIAN_PRIMARY','2030-02-03','https://catalog.example.test','Fixture only'),
		($2,'GB-ENG','Catalog Fixture','Fictional Framework','one','ENRICHMENT','2030-02-03','https://catalog.example.test','Fixture only'),
		($3,'NG','Catalog Fixture','Fictional Framework','two','NIGERIAN_PRIMARY','2030-02-03','https://catalog.example.test','Fixture only'),
		($4,'NG','Catalog Fixture','Fictional Framework','three','NIGERIAN_PRIMARY','2030-02-03','https://catalog.example.test','Fixture only')`,
		nigeria, enrichment, draft, withdrawn)
	write(`INSERT INTO curriculum_objectives (id,framework_version_id,stage,grade_band,subject,strand,code,objective_text)
		SELECT gen_random_uuid(),$1::uuid,CASE WHEN n = 52 THEN 'EARLY_YEARS' ELSE 'PRIMARY' END,
			'Fixture band','Mathematics','Number','FIX.' || n,'Fictional objective'
		FROM generate_series(1,52) AS n
		UNION ALL SELECT gen_random_uuid(),$2::uuid,'PRIMARY','Fixture band','Mathematics','Number','DRAFT','Private fixture'
		UNION ALL SELECT gen_random_uuid(),$3::uuid,'PRIMARY','Fixture band','Mathematics','Number','WITHDRAWN','Retired fixture'`,
		nigeria, draft, withdrawn)
	write(`UPDATE curriculum_framework_versions SET status='PUBLISHED' WHERE id IN ($1,$2,$3)`, nigeria, enrichment, withdrawn)
	write(`UPDATE curriculum_framework_versions SET status='WITHDRAWN' WHERE id=$1`, withdrawn)
	router, err := apphttp.NewRouter(apphttp.RouterConfig{Config: &config.Config{Environment: "test"}, DB: &db.DB{Pool: pool}})
	if err != nil {
		t.Fatal(err)
	}
	type item struct {
		ID            uuid.UUID
		FrameworkKind string `json:"framework_kind"`
		EffectiveFrom string `json:"effective_from"`
		LicenseNote   string `json:"license_note"`
		Stage         string
	}
	get := func(path, token string, want int) ([]item, string) {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/v1/curriculum/"+path, nil)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("curriculum status %d, want %d", rec.Code, want)
		}
		if want != 200 {
			return nil, ""
		}
		var result struct {
			Data struct {
				Items []item
				Next  string
			}
		}
		if rec.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(rec.Body.Bytes(), &result) != nil || result.Data.Items == nil {
			t.Fatal("invalid curriculum response")
		}
		return result.Data.Items, result.Data.Next
	}
	get("frameworks", "", 401)
	get("frameworks", operatorToken, 403)
	get("frameworks?after=invalid", memberToken, 422)
	get("objectives", memberToken, 422)
	get("objectives?framework_version_id="+nigeria.String()+"&stage=invalid", memberToken, 422)

	frameworks, _ := get("frameworks", memberToken, 200)
	if len(frameworks) != 2 {
		t.Fatal("expected only the two published editions")
	}
	known := map[uuid.UUID]string{nigeria: "NIGERIAN_PRIMARY", enrichment: "ENRICHMENT"}
	for _, value := range frameworks {
		if known[value.ID] != value.FrameworkKind || value.EffectiveFrom != "2030-02-03" || value.LicenseNote != "Fixture only" {
			t.Fatal("published edition metadata differs from the fixture")
		}
		delete(known, value.ID)
	}
	for _, id := range []uuid.UUID{draft, withdrawn, uuid.New()} {
		items, _ := get("objectives?framework_version_id="+id.String(), memberToken, 200)
		if len(items) != 0 {
			t.Fatal("non-published framework disclosed objective data")
		}
	}

	filter := "objectives?framework_version_id=" + nigeria.String() + "&stage=PRIMARY"
	items, next := get(filter, memberToken, 200)
	if len(items) != 50 || next != items[49].ID.String() || items[0].Stage != "PRIMARY" {
		t.Fatal("invalid first page of filtered objectives")
	}
	last, end := get(filter+"&after="+next, memberToken, 200)
	if len(last) != 1 || last[0].ID.String() <= next || end != "" {
		t.Fatal("invalid last page of filtered objectives")
	}
	write(`UPDATE tenants SET status='SUSPENDED' WHERE id=$1`, school)
	get("frameworks", memberToken, 403)
	write(`UPDATE tenants SET status='ACTIVE' WHERE id=$1`, school)
	write(`UPDATE memberships SET status='SUSPENDED' WHERE tenant_id=$1 AND user_id=$2`, school, member)
	get(filter, memberToken, 403)
}
