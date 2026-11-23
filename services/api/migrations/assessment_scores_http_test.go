package migrations

import (
	"context"
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

func testAssessmentScoreHTTP(t *testing.T, ctx context.Context, pool *pgxpool.Pool, school, teacher, offering, component, placement uuid.UUID) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	token, csrf := uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at)
		VALUES ($1,$2,$3,'127.0.0.1','Fixture',clock_timestamp()+interval '1 hour')`,
		teacher, auth.HashToken(token), auth.HashToken(csrf))
	router, err := apphttp.NewRouter(apphttp.RouterConfig{Config: &config.Config{Environment: "test"}, DB: &db.DB{Pool: pool}})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/tenants/lesson-fixture-a/teaching/offerings/" + offering.String() + "/components/" + component.String() + "/scores"
	send := func(method, path, credential, header, body string, want int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if credential != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: credential})
		}
		req.Header.Set(auth.HeaderCSRF, header)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("assessment score %s: got HTTP %d, want %d, response %s", method, rec.Code, want, rec.Body)
		}
		return rec.Body.String()
	}
	const initial = `{"score_hundredths":1925,"revision":0}`
	target := path + "/" + placement.String()
	send("GET", path, "", "", "", 401)
	send("PUT", target, token, "", initial, 403)
	send("PUT", strings.Replace(target, "lesson-fixture-a", "lesson-fixture-b", 1), token, csrf, initial, 404)
	send("PUT", path+"/"+uuid.NewString(), token, csrf, initial, 404)
	send("PUT", target, token, csrf, `{"score_hundredths":2001,"revision":0}`, 422)
	body := send("GET", path, token, "", "", 200)
	if !strings.Contains(body, placement.String()) || !strings.Contains(body, `"maximum_hundredths":2000`) {
		t.Fatal("authorized roster did not return the placement and component maximum")
	}
	send("PUT", target, token, csrf, initial, 200)
	send("PUT", target, token, csrf, initial, 409)
	send("PUT", target, token, csrf, `{"score_hundredths":2000,"revision":1}`, 200)
	body = send("GET", path, token, "", "", 200)
	if !strings.Contains(body, `"score_hundredths":2000,"revision":2`) {
		t.Fatal("roster did not return the persisted score and revision")
	}
	body = send("GET", strings.Replace(path, "lesson-fixture-a", "lesson-fixture-b", 1), token, "", "", 200)
	if !strings.Contains(body, `"items":[]`) {
		t.Fatal("cross-tenant roster disclosed score data")
	}
	exec(`UPDATE teacher_assignments SET status='INACTIVE' WHERE tenant_id=$1 AND offering_id=$2 AND status='ACTIVE'`, school, offering)
	send("PUT", target, token, csrf, `{"score_hundredths":0,"revision":2}`, 404)
	if body := send("GET", path, token, "", "", 200); !strings.Contains(body, `"items":[]`) {
		t.Fatal("revoked assignment disclosed score data")
	}
	var audits int
	var previous int
	err = pool.QueryRow(ctx, `SELECT count(*), max((metadata_redacted->>'previous_hundredths')::integer)
		FROM audit_events WHERE tenant_id=$1 AND actor_id=$2 AND action='assessment.score.saved'`,
		school, teacher).Scan(&audits, &previous)
	if err != nil || audits != 2 || previous != 1925 {
		t.Fatalf("expected two score audits with the previous score, got %d, %d, %v", audits, previous, err)
	}
}
