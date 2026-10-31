package database

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type Migration struct {
	Version int
	Up      string
}

const schemaMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    module TEXT NOT NULL,
    version INT NOT NULL,
    PRIMARY KEY (module, version)
)`

func Migrate(ctx context.Context, db *sqlx.DB, module string, migrations []Migration) error {
	if len(migrations) == 0 {
		return fmt.Errorf("database: module %s has no migrations", module)
	}
	if _, err := db.ExecContext(ctx, schemaMigrationsTable); err != nil {
		return fmt.Errorf("database: create schema_migrations: %w", err)
	}

	var current int
	if err := db.GetContext(ctx, &current,
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE module = $1`, module); err != nil {
		return fmt.Errorf("database: read migration version for %s: %w", module, err)
	}

	latest := 0
	for _, m := range migrations {
		if m.Version > latest {
			latest = m.Version
		}
	}
	if current > latest {
		return fmt.Errorf("database: module %s is at migration %d, but this build only supports %d",
			module, current, latest)
	}

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("database: begin migration %s to %d: %w", module, m.Version, err)
		}
		if _, err := tx.ExecContext(ctx, m.Up); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("database: migrate %s to %d: %w", module, m.Version, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (module, version) VALUES ($1, $2)`, module, m.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("database: stamp migration %s at %d: %w", module, m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("database: commit migration %s to %d: %w", module, m.Version, err)
		}
		current = m.Version
	}
	return nil
}
