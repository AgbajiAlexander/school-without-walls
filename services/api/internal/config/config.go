package config

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
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
	TrustedProxyCIDRs  []string
	TrustedProxyNets   []*net.IPNet
	PublicWebURL       string
	SMTPHost           string
	SMTPPort           int
	SMTPUsername       string
	SMTPPassword       string
	SMTPFrom           string
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
		return nil, fmt.Errorf("invalid SERVER_ENV: %s", env)
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
			if trimmed == "*" {
				return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS must not contain wildcard origins when credentials are enabled")
			}
			origins = append(origins, trimmed)
		}
	}
	trustedProxies := splitList(getEnv("TRUSTED_PROXY_CIDRS", ""))
	trustedProxyNets := make([]*net.IPNet, 0, len(trustedProxies))
	for _, cidr := range trustedProxies {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid TRUSTED_PROXY_CIDRS entry: %s", cidr)
		}
		trustedProxyNets = append(trustedProxyNets, network)
	}
	publicWebURL := strings.TrimRight(getEnv("PUBLIC_WEB_URL", "http://localhost:3000"), "/")
	webURL, err := url.ParseRequestURI(publicWebURL)
	if err != nil || webURL.Host == "" || (webURL.Scheme != "https" && !(env == "development" && webURL.Scheme == "http")) {
		return nil, fmt.Errorf("invalid PUBLIC_WEB_URL for environment %s", env)
	}
	smtpPort, err := strconv.Atoi(getEnv("SMTP_PORT", "587"))
	if err != nil || smtpPort <= 0 || smtpPort > 65535 {
		return nil, fmt.Errorf("invalid SMTP_PORT: %s", getEnv("SMTP_PORT", "587"))
	}
	smtpHost := getEnv("SMTP_HOST", "")
	smtpUsername := getEnv("SMTP_USERNAME", "")
	smtpPassword := getEnv("SMTP_PASSWORD", "")
	smtpFrom := getEnv("SMTP_FROM", "")
	if smtpHost != "" && smtpFrom == "" {
		return nil, fmt.Errorf("SMTP_FROM is required when SMTP_HOST is configured")
	}
	if smtpHost != "" {
		sender, err := mail.ParseAddress(smtpFrom)
		if err != nil || sender.Address == "" {
			return nil, fmt.Errorf("invalid SMTP_FROM address")
		}
	}
	if (smtpUsername == "") != (smtpPassword == "") {
		return nil, fmt.Errorf("SMTP_USERNAME and SMTP_PASSWORD must be configured together")
	}

	return &Config{
		Port:               port,
		Environment:        env,
		DatabaseURL:        dbURL,
		DatabaseMaxConns:   maxConns,
		DatabaseMinConns:   minConns,
		CORSAllowedOrigins: origins,
		TrustedProxyCIDRs:  trustedProxies,
		TrustedProxyNets:   trustedProxyNets,
		PublicWebURL:       publicWebURL,
		SMTPHost:           smtpHost,
		SMTPPort:           smtpPort,
		SMTPUsername:       smtpUsername,
		SMTPPassword:       smtpPassword,
		SMTPFrom:           smtpFrom,
		ShutdownTimeout:    10 * time.Second,
	}, nil
}

func splitList(value string) []string {
	items := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}
