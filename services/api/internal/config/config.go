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

	"github.com/jackc/pgx/v5/pgxpool"
)

type StaffInvitationLimits struct {
	PerActor  int
	PerTenant int
	Window    time.Duration
}

func DefaultStaffInvitationLimits() StaffInvitationLimits {
	return StaffInvitationLimits{PerActor: 10, PerTenant: 25, Window: time.Hour}
}

type MediaConfig struct {
	Endpoint, Bucket, Region, AccessKey, SecretKey string
	ScannerNetwork, ScannerAddress                 string
}

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
	StaffInvitations   StaffInvitationLimits
	LessonMedia        *MediaConfig
}

// Load loads configuration from environment variables with defaults for
// non-secret settings. The authoritative database URL must be explicit.
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

	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	databaseURL, err := url.Parse(dbURL)
	if err != nil || (databaseURL.Scheme != "postgres" && databaseURL.Scheme != "postgresql") ||
		databaseURL.Hostname() == "" || strings.Trim(databaseURL.Path, "/") == "" {
		return nil, fmt.Errorf("DATABASE_URL must be a valid PostgreSQL connection URI")
	}
	if _, err := pgxpool.ParseConfig(dbURL); err != nil {
		return nil, fmt.Errorf("DATABASE_URL must be a valid PostgreSQL connection URI")
	}

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
	for _, origin := range strings.Split(corsOriginsStr, ",") {
		trimmed := strings.TrimSpace(origin)
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

	limits, err := loadStaffInvitationLimits()
	if err != nil {
		return nil, err
	}
	media, err := loadMediaConfig()
	if err != nil {
		return nil, err
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
		StaffInvitations:   limits,
		LessonMedia:        media,
	}, nil
}

func loadStaffInvitationLimits() (StaffInvitationLimits, error) {
	defaults := DefaultStaffInvitationLimits()
	read := func(name string, fallback, max int) (int, error) {
		n, err := strconv.Atoi(getEnv(name, strconv.Itoa(fallback)))
		if err != nil || n < 1 || n > max {
			return 0, fmt.Errorf("%s must be an integer between 1 and %d", name, max)
		}
		return n, nil
	}
	actor, err := read("STAFF_INVITATION_ACTOR_LIMIT", defaults.PerActor, 1000)
	if err != nil {
		return StaffInvitationLimits{}, err
	}
	tenant, err := read("STAFF_INVITATION_TENANT_LIMIT", defaults.PerTenant, 1000)
	if err != nil {
		return StaffInvitationLimits{}, err
	}
	minutes, err := read("STAFF_INVITATION_WINDOW_MINUTES", int(defaults.Window/time.Minute), 1440)
	if err != nil {
		return StaffInvitationLimits{}, err
	}
	return StaffInvitationLimits{PerActor: actor, PerTenant: tenant, Window: time.Duration(minutes) * time.Minute}, nil
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

func loadMediaConfig() (*MediaConfig, error) {
	if getEnv("LESSON_MEDIA_ENABLED", "false") == "false" {
		return nil, nil
	}
	if os.Getenv("LESSON_MEDIA_ENABLED") != "true" {
		return nil, fmt.Errorf("invalid LESSON_MEDIA_ENABLED")
	}
	m := &MediaConfig{
		Endpoint: os.Getenv("LESSON_MEDIA_S3_ENDPOINT"), Bucket: os.Getenv("LESSON_MEDIA_S3_BUCKET"),
		Region: os.Getenv("LESSON_MEDIA_S3_REGION"), AccessKey: os.Getenv("LESSON_MEDIA_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("LESSON_MEDIA_S3_SECRET_KEY"), ScannerNetwork: os.Getenv("LESSON_MEDIA_SCANNER_NETWORK"),
		ScannerAddress: os.Getenv("LESSON_MEDIA_SCANNER_ADDRESS"),
	}
	if m.Endpoint == "" || m.Bucket == "" || m.Region == "" || m.AccessKey == "" || m.SecretKey == "" || m.ScannerAddress == "" {
		return nil, fmt.Errorf("lesson media requires private object storage and scanner configuration")
	}
	switch m.ScannerNetwork {
	case "unix":
		if !strings.HasPrefix(m.ScannerAddress, "/") {
			return nil, fmt.Errorf("lesson scanner requires an absolute socket path")
		}
	case "tcp":
		host, port, err := net.SplitHostPort(m.ScannerAddress)
		ip := net.ParseIP(host)
		n, portErr := strconv.Atoi(port)
		if err != nil || ip == nil || (!ip.IsLoopback() && !ip.IsPrivate()) || portErr != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("lesson scanner requires a private IP and port")
		}
	default:
		return nil, fmt.Errorf("invalid LESSON_MEDIA_SCANNER_NETWORK")
	}
	return m, nil
}
