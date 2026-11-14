package config

import (
	"strings"
	"testing"
)

func TestConfigLoadDefaultsRequireExplicitDatabaseURL(t *testing.T) {
	setDefaultTestEnvironment(t)

	if _, err := Load(); err == nil {
		t.Fatal("expected missing DATABASE_URL to fail")
	}

	t.Setenv("DATABASE_URL", "postgres://test-user:test-password@127.0.0.1:5432/sww_test?sslmode=disable")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected valid explicit database configuration, got %v", err)
	}
	if cfg.Port != 8080 || cfg.Environment != "development" {
		t.Fatalf("unexpected defaults: port=%d environment=%s", cfg.Port, cfg.Environment)
	}
	if cfg.DatabaseMaxConns != 25 || cfg.DatabaseMinConns != 5 {
		t.Fatalf("unexpected database pool defaults: max=%d min=%d", cfg.DatabaseMaxConns, cfg.DatabaseMinConns)
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:3000" {
		t.Fatalf("unexpected CORS origins: %v", cfg.CORSAllowedOrigins)
	}
}

func TestConfigLoadCustom(t *testing.T) {
	setDefaultTestEnvironment(t)
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("SERVER_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://test-user:test-password@localhost:5432/sww_prod")
	t.Setenv("DATABASE_MAX_CONNS", "50")
	t.Setenv("DATABASE_MIN_CONNS", "10")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://sww.example.com, https://admin.sww.example.com")
	t.Setenv("PUBLIC_WEB_URL", "https://sww.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected valid configuration, got %v", err)
	}
	if cfg.Port != 9090 || cfg.Environment != "production" {
		t.Fatalf("unexpected custom server configuration: port=%d environment=%s", cfg.Port, cfg.Environment)
	}
	if cfg.DatabaseMaxConns != 50 || cfg.DatabaseMinConns != 10 {
		t.Fatalf("unexpected custom connection limits: max=%d min=%d", cfg.DatabaseMaxConns, cfg.DatabaseMinConns)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("expected 2 CORS origins, got %d", len(cfg.CORSAllowedOrigins))
	}
}

func TestConfigInvalidPort(t *testing.T) {
	setDefaultTestEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://test-user:test-password@localhost:5432/sww_test")
	t.Setenv("SERVER_PORT", "invalid")

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid port to fail")
	}
}

func TestConfigRejectsInvalidEnvironmentAndWildcardCredentialedCORS(t *testing.T) {
	setDefaultTestEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://test-user:test-password@localhost:5432/sww_test")
	t.Setenv("SERVER_ENV", "prod")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid environment to fail closed")
	}
	t.Setenv("SERVER_ENV", "development")
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	if _, err := Load(); err == nil {
		t.Fatal("expected wildcard credentialed CORS origin to be rejected")
	}
}

func TestConfigRejectsInvalidDatabaseURLsWithoutEchoingThem(t *testing.T) {
	setDefaultTestEnvironment(t)
	for _, databaseURL := range []string{
		"",
		"not-a-postgres-uri",
		"https://database.invalid/sww",
		"postgres:///missing-host",
		"postgres://localhost/",
	} {
		t.Run(databaseURL, func(t *testing.T) {
			t.Setenv("DATABASE_URL", databaseURL)
			if _, err := Load(); err == nil {
				t.Fatal("expected invalid database configuration to fail")
			} else if databaseURL != "" && strings.Contains(err.Error(), databaseURL) {
				t.Fatal("configuration error must not echo the database URL")
			}
		})
	}
}

func setDefaultTestEnvironment(t *testing.T) {
	t.Helper()
	for key, value := range map[string]string{
		"SERVER_PORT": "", "SERVER_ENV": "", "DATABASE_URL": "",
		"DATABASE_MAX_CONNS": "", "DATABASE_MIN_CONNS": "",
		"CORS_ALLOWED_ORIGINS": "", "TRUSTED_PROXY_CIDRS": "",
		"PUBLIC_WEB_URL": "", "SMTP_HOST": "", "SMTP_PORT": "",
		"SMTP_USERNAME": "", "SMTP_PASSWORD": "", "SMTP_FROM": "",
	} {
		t.Setenv(key, value)
	}
}
