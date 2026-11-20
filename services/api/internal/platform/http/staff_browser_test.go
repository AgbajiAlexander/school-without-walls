package http

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/db"
	"github.com/school-without-walls/api/migrations"
)

// TestStaffBrowserServer is started explicitly by Playwright, outside the normal Go suite.
func TestStaffBrowserServer(t *testing.T) {
	dsn := os.Getenv("SWW_B12_DATABASE_URL")
	if dsn == "" {
		t.Skip("staff browser fixture is not enabled")
	}
	if dsn != "postgres://sww_browser@127.0.0.1:15432/sww_e2e_test?sslmode=disable" {
		t.Fatal("staff browser fixture requires the disposable loopback cluster")
	}
	if os.Getenv("SWW_B12_ADMIN_PASSWORD") == "" {
		t.Fatal("staff browser fixture requires an ephemeral bootstrap password")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:18080")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("cannot initialize staff browser database")
	}
	defer adminPool.Close()
	schema := "sww_browser_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal("cannot create isolated browser fixture schema")
	}
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid disposable browser database configuration")
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal("cannot initialize staff browser database")
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations (
		version varchar(64) PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		t.Fatal("cannot initialize isolated browser migration ledger")
	}
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatal(err)
	}
	service := auth.NewService(auth.NewPostgresRepository(pool))
	if _, err := service.BootstrapAdmin(ctx, "browser-platform@school.invalid", "Fictional Platform Operator",
		os.Getenv("SWW_B12_ADMIN_PASSWORD"), "staff-browser-bootstrap"); err != nil {
		t.Fatal("cannot bootstrap the fictional browser operator")
	}
	const webOrigin = "http://127.0.0.1:13000"
	handler, err := NewRouter(RouterConfig{
		Config: &config.Config{Environment: "development", PublicWebURL: webOrigin, CORSAllowedOrigins: []string{webOrigin}},
		DB:     &db.DB{Pool: pool},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatal(err)
	}
}
