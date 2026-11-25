package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type Migration struct {
	Version int
	Up      string
}

const schemaMigrationsTable = `
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    module TEXT NOT NULL,
    version INT NOT NULL,
    PRIMARY KEY (module, version)
)`

func Migrate(ctx context.Context, db *sqlx.DB, module string, migrations []Migration) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("database: begin migration %s: %w", module, err)
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil && !errors.Is(err, sql.ErrTxDone) {
			fmt.Printf("database: rollback migration %s: %v\n", module, err)
		}
	}(tx)
	if err := MigrateTx(ctx, tx, module, migrations); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("database: commit migration %s: %w", module, err)
	}
	return nil
}

func MigrateTx(ctx context.Context, tx *sqlx.Tx, module string, migrations []Migration) error {
	if len(migrations) == 0 {
		return fmt.Errorf("database: module %s has no migrations", module)
	}
	if _, err := tx.ExecContext(ctx, schemaMigrationsTable); err != nil {
		return fmt.Errorf("database: create schema_migrations: %w", err)
	}
	var current int
	if err := tx.GetContext(ctx, &current,
		`SELECT COALESCE(MAX(version), 0) FROM public.schema_migrations WHERE module = $1`, module); err != nil {
		return fmt.Errorf("database: read migration version for %s: %w", module, err)
	}
	latest := 0
	for _, migration := range migrations {
		latest = max(latest, migration.Version)
	}
	if current > latest {
		return fmt.Errorf("database: module %s is at migration %d, but this build only supports %d", module, current, latest)
	}
	for _, migration := range migrations {
		if migration.Version <= current {
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.Up); err != nil {
			return fmt.Errorf("database: migrate %s to %d: %w", module, migration.Version, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO public.schema_migrations (module, version) VALUES ($1, $2)`, module, migration.Version); err != nil {
			return fmt.Errorf("database: stamp migration %s at %d: %w", module, migration.Version, err)
		}
		current = migration.Version
	}
	return nil
}
