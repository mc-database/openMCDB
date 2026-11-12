package survey

import (
	_ "embed"

	"openmcdb/internal/database"
)

//goland:noinspection SqlResolve
var dbMigrations = []database.Migration{
	{
		Version: 1,
		Up: `
CREATE TABLE survey_snapshots (
	id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	schema BIGINT NOT NULL,
	first_request BOOLEAN NOT NULL,
	received_at BIGINT NOT NULL,
	mc_version TEXT,
	mod_version TEXT,
	environment TEXT,
	loader TEXT,
	loader_version TEXT,
	java_version TEXT,
	os_name TEXT,
	os_arch TEXT,
	cpu_cores BIGINT,
	cpu_model TEXT,
	gpu_model TEXT,
	memory_mb BIGINT,
	system_memory_mb BIGINT,
	vram_mb BIGINT,
	graphics_api TEXT,
	render_distance BIGINT,
	simulation_distance BIGINT,
	has_iris BOOLEAN,
	resolution_bucket TEXT,
	locale TEXT,
	mod_count BIGINT
);
`,
	},
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
