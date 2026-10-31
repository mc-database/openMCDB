package survey

import (
	_ "embed"

	"openmcdb/internal/database"
)

//go:embed schema.sql
var schemaSQL string

var dbMigrations = []database.Migration{
	{Version: 1, Up: schemaSQL},
	{
		Version: 2,
		Up: `
ALTER TABLE survey_snapshots
    ALTER COLUMN received_at TYPE TIMESTAMPTZ
    USING to_timestamp(received_at),
    ALTER COLUMN received_at SET NOT NULL,
    ALTER COLUMN received_at SET DEFAULT NOW()
`,
	},
}
