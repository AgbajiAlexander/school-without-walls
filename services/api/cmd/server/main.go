package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/school-without-walls/api/internal/config"
	"github.com/school-without-walls/api/internal/platform/db"
	apphttp "github.com/school-without-walls/api/internal/platform/http"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting school without walls api server")

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize database connection pool
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dbPool, err := db.Connect(ctx, db.Config{
		URL:      cfg.DatabaseURL,
		MaxConns: cfg.DatabaseMaxConns,
		MinConns: cfg.DatabaseMinConns,
	})
	if err != nil {
		slog.Warn("database connection failed on startup, server running in degraded mode", slog.Any("error", err))
	} else {
		defer dbPool.Close()
		slog.Info("connected to postgres database pool")
	}

	router := apphttp.NewRouter(apphttp.RouterConfig{
		Config: cfg,
		DB:     dbPool,
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("http server listening", slog.Int("port", cfg.Port), slog.String("env", cfg.Environment))
		serverErrors <- srv.ListenAndServe()
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped unexpectedly", slog.Any("error", err))
			os.Exit(1)
		}
	case sig := <-shutdown:
		slog.Info("shutdown signal received", slog.String("signal", sig.String()))

		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("graceful shutdown failed", slog.Any("error", err))
			_ = srv.Close()
		}
		slog.Info("server shutdown completed cleanly")
	}
}
