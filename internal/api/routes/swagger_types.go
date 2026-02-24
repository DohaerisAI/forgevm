package routes

// StatusResponse is a generic status response.
type StatusResponse struct {
	Status string `json:"status" example:"destroyed"`
}

// PruneResponse is the response from pruning sandboxes.
type PruneResponse struct {
	Pruned int `json:"pruned" example:"3"`
}

// HealthResponse is the response from the health check endpoint.
type HealthResponse struct {
	Status  string `json:"status" example:"ok"`
	Version string `json:"version" example:"1.0.0"`
	Uptime  string `json:"uptime" example:"2h30m15s"`
}

// MetricsResponse is the response from the metrics endpoint.
type MetricsResponse struct {
	SandboxesActive int    `json:"sandboxes_active" example:"5"`
	Providers       int    `json:"providers" example:"2"`
	Goroutines      int    `json:"goroutines" example:"12"`
	MemoryAllocMB   uint64 `json:"memory_alloc_mb" example:"64"`
	Uptime          string `json:"uptime" example:"2h30m15s"`
}

// TemplateSpawnOverrides are optional overrides when spawning from a template.
type TemplateSpawnOverrides struct {
	Provider string `json:"provider,omitempty" example:"firecracker"`
	TTL      string `json:"ttl,omitempty" example:"30m"`
}

// ConsoleLogResponse is the response from the console log endpoint.
type ConsoleLogResponse struct {
	Lines []string `json:"lines"`
}
