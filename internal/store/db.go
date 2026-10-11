package store

import (
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type DB struct {
	db *sqlx.DB
}

func Open(path string) (*DB, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA synchronous=NORMAL`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &DB{db: db}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}
