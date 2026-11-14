package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/school-without-walls/api/internal/config"
)

func TestInitializeRouterRefusesUnavailableDatabase(t *testing.T) {
	const databaseURL = "postgres://fictional:do-not-use@127.0.0.1:1/sww_test?connect_timeout=1"
	cfg := &config.Config{
		Environment:      "production",
		DatabaseURL:      databaseURL,
		DatabaseMaxConns: 1,
		DatabaseMinConns: 0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	router, pool, err := initializeRouter(ctx, cfg)
	if err == nil {
		if pool != nil {
			pool.Close()
		}
		t.Fatal("expected unavailable PostgreSQL to prevent server initialization")
	}
	if router != nil || pool != nil {
		t.Fatal("failed initialization must not return a router or open pool")
	}
	if strings.Contains(err.Error(), databaseURL) || strings.Contains(err.Error(), "do-not-use") {
		t.Fatal("startup error must not expose database credentials")
	}
}

func TestInitializeRouterRejectsMissingDatabaseConfiguration(t *testing.T) {
	cfg := &config.Config{Environment: "production"}
	if _, _, err := initializeRouter(context.Background(), cfg); err == nil {
		t.Fatal("expected missing database configuration to fail")
	}
}
