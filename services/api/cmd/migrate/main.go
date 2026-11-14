package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/school-without-walls/api/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	action := "up"
	if len(args) > 1 {
		return fmt.Errorf("usage: go run ./cmd/migrate [up|status]")
	}
	if len(args) == 1 {
		action = args[0]
	}
	if action != "up" && action != "status" {
		return fmt.Errorf("supported migration actions are up and status")
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("DATABASE_URL is not a valid PostgreSQL connection URI")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("could not initialize PostgreSQL connection")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("PostgreSQL is unavailable")
	}

	if action == "up" {
		if err := migrations.Up(ctx, pool); err != nil {
			return err
		}
		fmt.Println("All pending migrations applied.")
		return nil
	}

	statuses, err := migrations.ReadStatus(ctx, pool)
	if err != nil {
		return err
	}
	for _, status := range statuses {
		state := "pending"
		if status.Applied {
			state = "applied"
		}
		fmt.Printf("%s %s\n", state, status.Version)
	}
	return nil
}
