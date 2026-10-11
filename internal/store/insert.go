package store

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
)

var insertSnapshotQuery = buildInsertQuery("snapshots", reflect.TypeFor[Snapshot]())

func buildInsertQuery(table string, t reflect.Type) string {
	cols := make([]string, 0, t.NumField())
	for field := range t.Fields() {
		col := field.Tag.Get("db")
		if col == "" || col == "-" {
			continue
		}
		cols = append(cols, col)
	}
	if len(cols) == 0 {
		panic("store: no db columns for " + table)
	}
	params := make([]string, len(cols))
	for i, col := range cols {
		params[i] = ":" + col
	}

	return fmt.Sprintf("%s INTO %s (%s) VALUES (%s)", // ugly workaround so GoLand doesn't interpret this as an SQL query and complain about the %s
		"INSERT", table, strings.Join(cols, ", "), strings.Join(params, ", "))
}

func (d *DB) Insert(ctx context.Context, s Snapshot) error {
	if s.Schema == nil {
		return fmt.Errorf("%w: missing schema", ErrInvalidSnapshot)
	}
	if s.FirstRequest == nil {
		return fmt.Errorf("%w: missing first_request", ErrInvalidSnapshot)
	}
	now := time.Now().Unix()
	s.ReceivedAt = &now
	_, err := d.db.NamedExecContext(ctx, insertSnapshotQuery, s)
	return err
}

func (d *DB) Count(ctx context.Context) (int, error) {
	var n int
	err := d.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM snapshots`)
	return n, err
}
