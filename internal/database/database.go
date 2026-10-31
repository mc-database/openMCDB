package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func Open(ctx context.Context, connectionString string) (*sqlx.DB, error) {
	if strings.TrimSpace(connectionString) == "" {
		return nil, errors.New("database: OPENMCDB_DATABASE_URL is required")
	}

	db, err := sqlx.Open("pgx", connectionString)
	if err != nil {
		return nil, fmt.Errorf("database: open PostgreSQL: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("database: connect to PostgreSQL: %w", err)
	}
	return db, nil
}
