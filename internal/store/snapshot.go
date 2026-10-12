package store

type Snapshot struct {
	Schema             *int    `db:"schema" json:"schema"`
	FirstRequest       *bool   `db:"first_request" json:"first_request"`
	ReceivedAt         *int64  `db:"received_at" json:"-"`
	MCVersion          *string `db:"mc_version" json:"mc_version"`
	ModVersion         *string `db:"mod_version" json:"mod_version"`
	Environment        *string `db:"environment" json:"environment"`
	Loader             *string `db:"loader" json:"loader"`
	LoaderVersion      *string `db:"loader_version" json:"loader_version"`
	JavaVersion        *string `db:"java_version" json:"java_version"`
	OSName             *string `db:"os_name" json:"os_name"`
	OSArch             *string `db:"os_arch" json:"os_arch"`
	CPUCores           *int    `db:"cpu_cores" json:"cpu_cores"`
	CPUModel           *string `db:"cpu_model" json:"cpu_model"`
	GPUModel           *string `db:"gpu_model" json:"gpu_model"`
	MemoryMB           *int    `db:"memory_mb" json:"memory_mb"`
	SystemMemoryMB     *int    `db:"system_memory_mb" json:"system_memory_mb"`
	GraphicsAPI        *string `db:"graphics_api" json:"graphics_api"`
	RenderDistance     *int    `db:"render_distance" json:"render_distance"`
	SimulationDistance *int    `db:"simulation_distance" json:"simulation_distance"`
	HasIris            *bool   `db:"has_iris" json:"has_iris"`
	ResolutionBucket   *string `db:"resolution_bucket" json:"resolution_bucket"`
	Fullscreen         *bool   `db:"fullscreen" json:"fullscreen"`
	Locale             *string `db:"locale" json:"locale"`
	ModCount           *int    `db:"mod_count" json:"mod_count"`
}
