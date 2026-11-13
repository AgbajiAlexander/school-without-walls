package config

import (
	"os"
	"testing"
)

func TestConfigLoadDefaults(t *testing.T) {
	// Clear any environment variables for a clean test
	os.Unsetenv("SERVER_PORT")
	os.Unsetenv("SERVER_ENV")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("DATABASE_MAX_CONNS")
	os.Unsetenv("DATABASE_MIN_CONNS")
	os.Unsetenv("CORS_ALLOWED_ORIGINS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error loading default config, got %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Port)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected default env development, got %s", cfg.Environment)
	}
	if cfg.DatabaseMaxConns != 25 {
		t.Errorf("expected default max conns 25, got %d", cfg.DatabaseMaxConns)
	}
	if cfg.DatabaseMinConns != 5 {
		t.Errorf("expected default min conns 5, got %d", cfg.DatabaseMinConns)
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("unexpected CORS origins: %v", cfg.CORSAllowedOrigins)
	}
}

func TestConfigLoadCustom(t *testing.T) {
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("SERVER_ENV", "production")
	os.Setenv("DATABASE_URL", "postgres://test_user:secret@db:5432/sww_prod")
	os.Setenv("DATABASE_MAX_CONNS", "50")
	os.Setenv("DATABASE_MIN_CONNS", "10")
	os.Setenv("CORS_ALLOWED_ORIGINS", "https://sww.example.com, https://admin.sww.example.com")
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("SERVER_ENV")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("DATABASE_MAX_CONNS")
		os.Unsetenv("DATABASE_MIN_CONNS")
		os.Unsetenv("CORS_ALLOWED_ORIGINS")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.Environment != "production" {
		t.Errorf("expected env production, got %s", cfg.Environment)
	}
	if cfg.DatabaseMaxConns != 50 {
		t.Errorf("expected max conns 50, got %d", cfg.DatabaseMaxConns)
	}
	if cfg.DatabaseMinConns != 10 {
		t.Errorf("expected min conns 10, got %d", cfg.DatabaseMinConns)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Errorf("expected 2 CORS origins, got %d", len(cfg.CORSAllowedOrigins))
	}
}

func TestConfigInvalidPort(t *testing.T) {
	os.Setenv("SERVER_PORT", "invalid")
	defer os.Unsetenv("SERVER_PORT")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid port, got nil")
	}
}
