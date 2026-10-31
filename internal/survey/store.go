package survey

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"openmcdb/internal/database"
)

func Init(ctx context.Context, db *sqlx.DB) error {
	return database.Migrate(ctx, db, "survey", dbMigrations)
}

type store struct {
	db *sqlx.DB
}

const insertSnapshotQuery = `
INSERT INTO survey.snapshots (
    schema, first_request, mc_version, mod_version,
    environment, loader, loader_version, java_version, os_name, os_arch,
    cpu_cores, cpu_model, gpu_model, memory_mb, system_memory_mb, vram_mb,
    graphics_api, render_distance, simulation_distance, has_iris,
    resolution_bucket, locale, mod_count
) VALUES (
    :schema, :first_request, :mc_version, :mod_version,
    :environment, :loader, :loader_version, :java_version, :os_name, :os_arch,
    :cpu_cores, :cpu_model, :gpu_model, :memory_mb, :system_memory_mb, :vram_mb,
    :graphics_api, :render_distance, :simulation_distance, :has_iris,
    :resolution_bucket, :locale, :mod_count
)`

const queryTimeout = 5 * time.Second

func (s *store) Insert(ctx context.Context, snap Snapshot) error {
	if err := validateSnapshot(snap); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := s.db.NamedExecContext(ctx, insertSnapshotQuery, snap)
	return err
}

func (s *store) Count(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var count int64
	err := s.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM survey.snapshots`)
	return count, err
}
