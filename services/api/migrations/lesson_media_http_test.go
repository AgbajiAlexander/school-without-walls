package migrations

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/db"
	apphttp "github.com/school-without-walls/api/internal/platform/http"
)

func testLessonMediaHTTP(t *testing.T, ctx context.Context, pool *pgxpool.Pool, school, teacher, offering, objective, session, term, class uuid.UUID) {
	t.Helper()
	data := "%PDF-1.4\nfictional lesson fixture"
	sum := sha256.Sum256([]byte(data))
	checksum := base64.StdEncoding.EncodeToString(sum[:])
	var ready atomic.Bool
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/fixture/quarantine/"):
			_, _ = io.WriteString(w, data)
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/fixture/ready/"):
			if body, err := io.ReadAll(r.Body); err != nil || string(body) != data || r.Header.Get("If-None-Match") != "*" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			ready.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer storage.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	scanResult := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			scanResult <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		reader := bufio.NewReader(conn)
		command, err := reader.ReadString(0)
		if err != nil || command != "zINSTREAM\x00" {
			scanResult <- fmt.Errorf("unexpected scanner command")
			return
		}
		var size uint32
		if err := binary.Read(reader, binary.BigEndian, &size); err != nil || int(size) != len(data) {
			scanResult <- fmt.Errorf("unexpected scanner length")
			return
		}
		scanned := make([]byte, len(data))
		if _, err := io.ReadFull(reader, scanned); err != nil || string(scanned) != data {
			scanResult <- fmt.Errorf("unexpected scanner data")
			return
		}
		if err := binary.Read(reader, binary.BigEndian, &size); err != nil || size != 0 {
			scanResult <- fmt.Errorf("invalid scanner stream ending")
			return
		}
		_, err = io.WriteString(conn, "stream: OK\x00")
		scanResult <- err
	}()
	write := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	write(`UPDATE tenants SET feature_flags='{}' WHERE id=$1`, school)
	teacherToken, studentToken, reviewerToken, csrf := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	write(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at)
	SELECT id,$1,$2,'127.0.0.1','Fixture',clock_timestamp()+interval '1 hour' FROM users WHERE email='lesson-reviewer@example.test'`,
		auth.HashToken(reviewerToken), auth.HashToken(csrf))
	write(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at) VALUES
 ($1,$2,$3,'127.0.0.1','Fixture',clock_timestamp()+interval '1 hour')`, teacher, auth.HashToken(teacherToken), auth.HashToken(csrf))
	write(`INSERT INTO sessions (user_id,token_hash,csrf_token_hash,ip_address,user_agent,expires_at)
 SELECT user_id,$2,$3,'127.0.0.1','Fixture',clock_timestamp()+interval '1 hour' FROM students WHERE tenant_id=$1`,
		school, auth.HashToken(studentToken), auth.HashToken(csrf))
	router, err := apphttp.NewRouter(apphttp.RouterConfig{DB: &db.DB{Pool: pool}, Config: &config.Config{Environment: "test", LessonMedia: &config.MediaConfig{
		Endpoint: storage.URL, Bucket: "fixture", Region: "us-east-1", AccessKey: "dummy", SecretKey: "dummy",
		ScannerNetwork: "tcp", ScannerAddress: listener.Addr().String(),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	hit := func(method, path, token, header, body string, want int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		req.Header.Set(auth.HeaderCSRF, header)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("lesson media HTTP status %d, want %d", res.Code, want)
		}
		if want == 200 && res.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("media response must not be cached")
		}
		return res.Body.String()
	}
	parseID := func(body string) uuid.UUID {
		t.Helper()
		var payload struct{ Data struct{ ID uuid.UUID } }
		if err := json.Unmarshal([]byte(body), &payload); err != nil || payload.Data.ID == uuid.Nil {
			t.Fatal("missing media fixture identifier")
		}
		return payload.Data.ID
	}
	const root = "/api/v1/tenants/lesson-fixture-a"
	path := root + "/teaching/lessons"
	draft := fmt.Sprintf(`{"offering_id":%q,"primary_objective_id":%q,"title":"Media fixture","content":"Fictional text"}`, offering, objective)
	lesson := parseID(hit("POST", path, teacherToken, csrf, draft, 201))
	edit := path + "/" + lesson.String()
	if body := hit("GET", edit, teacherToken, "", "", 200); !strings.Contains(body, "\"media_upload_enabled\":true") {
		t.Fatal("configured draft must expose the upload capability to its author")
	}
	upload := edit + "/media"
	download := root + "/learning/lessons/" + lesson.String() + "/media"
	intent := fmt.Sprintf(`{"media_type":"application/pdf","size_bytes":%d,"checksum_sha256":%q}`, len(data), checksum)
	hit("POST", upload, teacherToken, "", intent, 403)
	hit("POST", upload, studentToken, csrf, intent, 404)
	hit("POST", strings.Replace(upload, "lesson-fixture-a", "lesson-fixture-b", 1), teacherToken, csrf, intent, 404)
	out := hit("POST", upload, teacherToken, csrf, intent, 201)
	id := parseID(out)
	if !strings.Contains(out, "/quarantine/") || ready.Load() {
		t.Fatal("upload must be quarantined")
	}
	hit("GET", download, studentToken, "", "", 404)
	hit("POST", upload+"/"+id.String()+"/complete", teacherToken, csrf, "", 200)
	if err := <-scanResult; err != nil || !ready.Load() {
		t.Fatalf("scan/promotion failed: %v", err)
	}
	reviews := root + "/lesson-reviews"
	review := reviews + "/" + lesson.String()
	attachment := review + "/media?revision="
	currentRevision := func() int64 {
		t.Helper()
		var result struct {
			Data struct {
				Items []struct {
					ID            uuid.UUID
					Revision      int64
					HasAttachment bool `json:"has_attachment"`
				}
			}
		}
		body := hit("GET", reviews, reviewerToken, "", "", 200)
		if strings.Contains(body, "X-Amz-") || json.Unmarshal([]byte(body), &result) != nil {
			t.Fatal("invalid review queue or signed URL exposed")
		}
		for _, item := range result.Data.Items {
			if item.ID == lesson && item.Revision > 0 && item.HasAttachment {
				return item.Revision
			}
		}
		t.Fatal("ready attachment missing from current pending review")
		return 0
	}
	hit("POST", edit+"/submit", teacherToken, csrf, "", 200)
	first := currentRevision()
	link := fmt.Sprintf("%s%d", attachment, first)
	hit("GET", link, "", "", "", 401)
	hit("GET", link, teacherToken, "", "", 404)
	hit("GET", link, studentToken, "", "", 404)
	hit("GET", strings.Replace(link, "lesson-fixture-a", "lesson-fixture-b", 1), reviewerToken, "", "", 404)
	hit("GET", review+"/media", reviewerToken, "", "", 404)
	if body := hit("GET", link, reviewerToken, "", "", 200); !strings.Contains(body, "/ready/") || !strings.Contains(body, "X-Amz-Expires=120") {
		t.Fatal("review grant did not restrict access to the ready prefix with short expiry")
	}
	write(`UPDATE memberships SET status='SUSPENDED' WHERE tenant_id=$1 AND user_id=(SELECT id FROM users WHERE email='lesson-reviewer@example.test')`, school)
	hit("GET", link, reviewerToken, "", "", 404)
	write(`UPDATE memberships SET status='ACTIVE' WHERE tenant_id=$1 AND user_id=(SELECT id FROM users WHERE email='lesson-reviewer@example.test')`, school)
	hit("PATCH", edit, teacherToken, csrf, `{"title":"Revised media fixture","content":"Fictional revision"}`, 200)
	hit("GET", link, reviewerToken, "", "", 404)
	hit("POST", edit+"/submit", teacherToken, csrf, "", 200)
	second := currentRevision()
	if second <= first {
		t.Fatal("edited review did not advance the revision")
	}
	hit("GET", link, reviewerToken, "", "", 404)
	newLink := fmt.Sprintf("%s%d", attachment, second)
	hit("GET", newLink, reviewerToken, "", "", 200)
	hit("POST", review+"/approve", reviewerToken, csrf, fmt.Sprintf(`{"revision":%d}`, second), 200)
	hit("GET", newLink, reviewerToken, "", "", 404)
	hit("POST", edit+"/publish", teacherToken, csrf, "", 200)
	if body := hit("GET", edit, teacherToken, "", "", 200); strings.Contains(body, "\"media_upload_enabled\":true") {
		t.Fatal("published lesson must not expose the upload capability")
	}
	hit("GET", download, studentToken, "", "", 404)
	enrollment := uuid.New()
	write(`INSERT INTO enrollments (id,tenant_id,student_id,session_id,term_id,class_id)
 SELECT $1,$2,id,$3,$4,$5 FROM students WHERE tenant_id=$2`, enrollment, school, session, term, class)
	detail := strings.TrimSuffix(download, "/media")
	if body := hit("GET", detail, studentToken, "", "", 200); !strings.Contains(body, "\"has_attachment\":true") || strings.Contains(body, "X-Amz-") {
		t.Fatal("lesson detail must advertise the attachment without a signed URL")
	}
	if body := hit("GET", download, studentToken, "", "", 200); !strings.Contains(body, "/ready/") || !strings.Contains(body, "X-Amz-Expires=120") {
		t.Fatal("download grant was missing ready scope or expiry")
	}
	hit("GET", strings.Replace(download, "lesson-fixture-a", "lesson-fixture-b", 1), studentToken, "", "", 404)
	write(`UPDATE lesson_media SET status='REJECTED' WHERE id=$1`, id)
	if body := hit("GET", detail, studentToken, "", "", 200); strings.Contains(body, "\"has_attachment\":true") {
		t.Fatal("rejected attachment must not be advertised")
	}
	hit("GET", download, studentToken, "", "", 404)
}
