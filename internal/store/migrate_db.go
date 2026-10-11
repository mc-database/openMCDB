package store

import (
	"fmt"

	"github.com/jmoiron/sqlx"

	"openmcdb/internal/store/migrations"
)

func migrateDB(db *sqlx.DB) error {
	var current int
	if err := db.Get(&current, `PRAGMA user_version`); err != nil {
		return fmt.Errorf("store: read user_version: %w", err)
	}

	has, err := hasTable(db, "snapshots")
	if err != nil {
		return fmt.Errorf("store: check snapshots table: %w", err)
	}
	if current == 0 && has {
		return fmt.Errorf("store: unversioned database v%d",
			migrations.MinSupportedSchema)
	}

	for _, m := range migrations.All() {
		if m.Version <= current {
			continue
		}
		tx, err := db.Beginx()
		if err != nil {
			return fmt.Errorf("store: begin migration to %d: %w", m.Version, err)
		}
		if _, err := tx.Exec(m.UpDB); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: migrate db to %d: %w", m.Version, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, m.Version)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: stamp user_version %d: %w", m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: commit migration to %d: %w", m.Version, err)
		}
		current = m.Version
	}
	return nil
}

func hasTable(db *sqlx.DB, name string) (bool, error) {
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name); err != nil {
		return false, err
	}
	return n > 0, nil
}
