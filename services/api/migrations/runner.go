package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.up.sql
var files embed.FS

const migrationLockID int64 = 752003003

type Migration struct {
	Version string
	SQL     string
}

type Status struct {
	Version string
	Applied bool
}

func All() ([]Migration, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations")
	}

	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		sql, err := files.ReadFile(path.Clean(entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %s", entry.Name())
		}
		migrations = append(migrations, Migration{
			Version: strings.TrimSuffix(entry.Name(), ".up.sql"),
			SQL:     string(sql),
		})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

func Up(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := All()
	if err != nil {
		return err
	}
	return Apply(ctx, pool, migrations)
}

func Apply(ctx context.Context, pool *pgxpool.Pool, migrations []Migration) error {
	if pool == nil {
		return fmt.Errorf("database pool is required")
	}

	for _, migration := range migrations {
		if migration.Version == "" || strings.TrimSpace(migration.SQL) == "" {
			return fmt.Errorf("migration version and SQL are required")
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s", migration.Version)
		}
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLockID); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("lock migration %s", migration.Version)
		}

		var ledgerExists bool
		if err := tx.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&ledgerExists); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("check migration ledger")
		}
		if ledgerExists {
			var applied bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (
				SELECT 1 FROM schema_migrations WHERE version = $1
			)`, migration.Version).Scan(&applied); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("read migration ledger")
			}
			if applied {
				if err := tx.Commit(ctx); err != nil {
					return fmt.Errorf("finish migration %s", migration.Version)
				}
				continue
			}
		}

		if _, err := tx.Exec(ctx, migration.SQL, pgx.QueryExecModeSimpleProtocol); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("execute migration %s", migration.Version)
		}
		if err := tx.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&ledgerExists); err != nil || !ledgerExists {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s did not create schema_migrations", migration.Version)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version)
			VALUES ($1) ON CONFLICT (version) DO NOTHING`, migration.Version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s", migration.Version)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s", migration.Version)
		}
	}
	return nil
}

func ReadStatus(ctx context.Context, pool *pgxpool.Pool) ([]Status, error) {
	if pool == nil {
		return nil, fmt.Errorf("database pool is required")
	}
	migrations, err := All()
	if err != nil {
		return nil, err
	}

	var ledgerExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&ledgerExists); err != nil {
		return nil, fmt.Errorf("read migration ledger status")
	}

	statuses := make([]Status, 0, len(migrations))
	for _, migration := range migrations {
		status := Status{Version: migration.Version}
		if ledgerExists {
			if err := pool.QueryRow(ctx, `SELECT EXISTS (
				SELECT 1 FROM schema_migrations WHERE version = $1
			)`, migration.Version).Scan(&status.Applied); err != nil {
				return nil, fmt.Errorf("read migration ledger status")
			}
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}
