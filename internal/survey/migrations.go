package survey

import (
	_ "embed"

	"openmcdb/internal/database"
)

//go:embed schema.sql
var schemaSQL string

var dbMigrations = []database.Migration{
	{Version: 1, Up: schemaSQL},
}
