package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"openmcdb/internal/store/migrations"
)

var (
	ErrSchemaMissing   = errors.New("missing schema")
	ErrSchemaTooOld    = errors.New("unsupported schema: too old")
	ErrSchemaTooNew    = errors.New("unsupported schema: too new")
	ErrInvalidSnapshot = errors.New("invalid snapshot")
)

func UpgradeToLatest(raw []byte) (Snapshot, int, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return Snapshot{}, 0, err
	}
	from, err := parseSchemaField(m["schema"])
	if err != nil {
		return Snapshot{}, 0, err
	}
	switch {
	case from < migrations.MinSupportedSchema:
		return Snapshot{}, from, fmt.Errorf("%w: got %d, minimum %d",
			ErrSchemaTooOld, from, migrations.MinSupportedSchema)
	case from > migrations.Latest():
		return Snapshot{}, from, fmt.Errorf("%w: got %d, latest %d",
			ErrSchemaTooNew, from, migrations.Latest())
	}

	for v := from + 1; v <= migrations.Latest(); v++ {
		mig, ok := migrations.ByVersion(v)
		if !ok {
			return Snapshot{}, from, fmt.Errorf("store: no migration for properties %d", v)
		}
		if mig.UpPayload != nil {
			mig.UpPayload(m)
		}
		m["schema"] = v
	}

	upgraded, err := json.Marshal(m)
	if err != nil {
		return Snapshot{}, from, err
	}
	var snap Snapshot
	if err := json.Unmarshal(upgraded, &snap); err != nil {
		return Snapshot{}, from, err
	}
	orig := from
	snap.Schema = &orig
	if snap.FirstRequest == nil {
		return Snapshot{}, from, fmt.Errorf("%w: missing first_request", ErrInvalidSnapshot)
	}
	return snap, from, nil
}

func parseSchemaField(v any) (int, error) {
	switch n := v.(type) {
	case nil:
		return 0, ErrSchemaMissing
	case float64:
		if math.Trunc(n) != n {
			return 0, fmt.Errorf("%w: must be an integer, got %v", ErrSchemaMissing, n)
		}
		return int(n), nil
	default:
		return 0, ErrSchemaMissing
	}
}
