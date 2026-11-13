package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pinger defines an interface for components that can be health-checked via Ping.
type Pinger interface {
	Ping(ctx context.Context) error
}

// DB wraps pgxpool.Pool.
type DB struct {
	Pool *pgxpool.Pool
}

// Config holds DB connection settings.
type Config struct {
	URL      string
	MaxConns int
	MinConns int
}

// Connect creates a new pgx connection pool with configuration.
func Connect(ctx context.Context, cfg Config) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = int32(cfg.MaxConns)
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = int32(cfg.MinConns)
	}
	poolCfg.MaxConnLifetime = 1 * time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection pool: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Ping checks database connectivity.
func (d *DB) Ping(ctx context.Context) error {
	if d == nil || d.Pool == nil {
		return fmt.Errorf("database pool is not initialized")
	}
	return d.Pool.Ping(ctx)
}

// Close closes the pool.
func (d *DB) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
