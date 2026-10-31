package survey

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

const MinSupportedSchema = 1

var (
	ErrUnsupportedSchema = errors.New("unsupported schema")
	ErrInvalidSnapshot   = errors.New("invalid snapshot")
)

type payloadMigration struct {
	version int
	up      func(map[string]any)
}

var payloadMigrations = []payloadMigration{
	{version: 1, up: nil},
}

func latestSchema() int {
	return payloadMigrations[len(payloadMigrations)-1].version
}

func init() {
	if len(payloadMigrations) == 0 {
		panic("survey: empty payload chain")
	}
	for i, m := range payloadMigrations {
		if want := MinSupportedSchema + i; m.version != want {
			panic(fmt.Sprintf("survey: payload entry %d has version %d, want %d", i, m.version, want))
		}
	}
}

func decodeSnapshot(raw []byte) (Snapshot, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	from, err := parseSchemaField(m["schema"])
	if err != nil {
		return Snapshot{}, err
	}
	switch {
	case from < MinSupportedSchema:
		return Snapshot{}, fmt.Errorf("%w: too old", ErrUnsupportedSchema)
	case from > latestSchema():
		return Snapshot{}, fmt.Errorf("%w: too new", ErrUnsupportedSchema)
	}

	for v := from + 1; v <= latestSchema(); v++ {
		mig := payloadMigrations[v-MinSupportedSchema]
		if mig.up != nil {
			mig.up(m)
		}
		m["schema"] = v
	}

	normalized, err := json.Marshal(m)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	var snap Snapshot
	if err := json.Unmarshal(normalized, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	snap.Schema = &from
	if err := validateSnapshot(snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

func parseSchemaField(v any) (int, error) {
	switch n := v.(type) {
	case nil:
		return 0, fmt.Errorf("%w: missing", ErrUnsupportedSchema)
	case float64:
		if math.Trunc(n) != n {
			return 0, fmt.Errorf("%w: not an integer", ErrUnsupportedSchema)
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("%w: not a number", ErrUnsupportedSchema)
	}
}

func validateSnapshot(snap Snapshot) error {
	if snap.Schema == nil {
		return fmt.Errorf("%w: missing schema", ErrInvalidSnapshot)
	}
	if snap.FirstRequest == nil {
		return fmt.Errorf("%w: missing first_request", ErrInvalidSnapshot)
	}
	for _, value := range []*string{
		snap.MCVersion, snap.ModVersion, snap.Environment, snap.Loader,
		snap.LoaderVersion, snap.JavaVersion, snap.OSName, snap.OSArch,
		snap.CPUModel, snap.GPUModel, snap.GraphicsAPI,
		snap.ResolutionBucket, snap.Locale,
	} {
		if value != nil && strings.ContainsRune(*value, '\x00') {
			return fmt.Errorf("%w: string contains a NUL byte", ErrInvalidSnapshot)
		}
	}
	return nil
}
