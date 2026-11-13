package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all backend runtime configuration.
type Config struct {
	Port               int
	Environment        string
	DatabaseURL        string
	DatabaseMaxConns   int
	DatabaseMinConns   int
	CORSAllowedOrigins []string
	ShutdownTimeout    time.Duration
}

// Load loads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	portStr := getEnv("SERVER_PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid SERVER_PORT: %s", portStr)
	}

	env := strings.ToLower(getEnv("SERVER_ENV", "development"))
	if env != "development" && env != "staging" && env != "production" && env != "test" {
		env = "development"
	}

	dbURL := getEnv("DATABASE_URL", "postgres://sww_user:sww_password@localhost:5432/sww_dev?sslmode=disable")

	maxConnsStr := getEnv("DATABASE_MAX_CONNS", "25")
	maxConns, err := strconv.Atoi(maxConnsStr)
	if err != nil || maxConns <= 0 {
		maxConns = 25
	}

	minConnsStr := getEnv("DATABASE_MIN_CONNS", "5")
	minConns, err := strconv.Atoi(minConnsStr)
	if err != nil || minConns < 0 {
		minConns = 5
	}

	corsOriginsStr := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	var origins []string
	for _, o := range strings.Split(corsOriginsStr, ",") {
		trimmed := strings.TrimSpace(o)
		if trimmed != "" {
			origins = append(origins, trimmed)
		}
	}

	return &Config{
		Port:               port,
		Environment:        env,
		DatabaseURL:        dbURL,
		DatabaseMaxConns:   maxConns,
		DatabaseMinConns:   minConns,
		CORSAllowedOrigins: origins,
		ShutdownTimeout:    10 * time.Second,
	}, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}
