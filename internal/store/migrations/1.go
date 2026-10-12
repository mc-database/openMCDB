package migrations

var V1 = Migration{
	Version: 1,
	UpDB: `
CREATE TABLE snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    schema INTEGER NOT NULL,
    first_request INTEGER NOT NULL,
    received_at INTEGER NOT NULL,
    mc_version TEXT,
    mod_version TEXT,
    environment TEXT,
    loader TEXT,
    loader_version TEXT,
    java_version TEXT,
    os_name TEXT,
    os_arch TEXT,
    cpu_cores INTEGER,
    cpu_model TEXT,
    gpu_model TEXT,
    memory_mb INTEGER,
    system_memory_mb INTEGER,
    vram_mb INTEGER,
    graphics_api TEXT,
    render_distance INTEGER,
    simulation_distance INTEGER,
    has_iris INTEGER,
    resolution_bucket TEXT,
    locale TEXT,
    mod_count INTEGER
);
`,
	UpPayload: nil,
}
