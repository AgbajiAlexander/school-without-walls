package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/modules/auth"
	"github.com/school-without-walls/api/internal/platform/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	emailFlag := flag.String("email", os.Getenv("BOOTSTRAP_ADMIN_EMAIL"), "Initial platform administrator email")
	nameFlag := flag.String("name", os.Getenv("BOOTSTRAP_ADMIN_NAME"), "Initial platform administrator name")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", slog.Any("error", err))
		os.Exit(1)
	}
	if cfg.Environment != "development" || os.Getenv("SWW_ENABLE_DEV_BOOTSTRAP") != "true" {
		slog.Error("bootstrap denied: this command requires development mode and SWW_ENABLE_DEV_BOOTSTRAP=true")
		os.Exit(1)
	}

	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if strings.TrimSpace(*emailFlag) == "" || strings.TrimSpace(*nameFlag) == "" || password == "" {
		slog.Error("bootstrap requires BOOTSTRAP_ADMIN_EMAIL, BOOTSTRAP_ADMIN_NAME, and BOOTSTRAP_ADMIN_PASSWORD")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dbPool, err := db.Connect(ctx, db.Config{
		URL: cfg.DatabaseURL, MaxConns: 5, MinConns: 1,
	})
	if err != nil {
		slog.Error("failed to connect to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer dbPool.Close()

	repo := auth.NewPostgresRepository(dbPool.Pool)
	svc := auth.NewService(repo)
	admin, err := svc.BootstrapAdmin(ctx, *emailFlag, *nameFlag, password, "bootstrap-cli")
	if err != nil {
		slog.Error("failed to bootstrap platform administrator", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("platform administrator bootstrapped successfully",
		slog.String("id", admin.ID.String()),
	)
}
