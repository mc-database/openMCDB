package survey

import (
	_ "embed"

	"openmcdb/internal/database"
)

//go:embed schema.sql
var schemaSQL string

//goland:noinspection SqlResolve
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
	{
		Version: 3,
		Up: `
CREATE SCHEMA IF NOT EXISTS survey;
ALTER TABLE survey_snapshots SET SCHEMA survey;
ALTER TABLE survey.survey_snapshots RENAME TO snapshots;
`,
	},
}
