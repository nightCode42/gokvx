package config

// Observability configures logs, traces, metrics, and health (spec §12).
type Observability struct {
	// Log configures structured logging.
	Log Log `koanf:"log"`
	// Tracing configures OpenTelemetry tracing.
	Tracing Tracing `koanf:"tracing"`
	// Metrics configures Prometheus metrics.
	Metrics Metrics `koanf:"metrics"`
	// Health configures the readiness check.
	Health Health `koanf:"health"`
}

// Log configures structured logging (KV-OBS-020 to KV-OBS-024).
type Log struct {
	// Level is the minimum level: debug, info, warn, or error.
	Level string `koanf:"level"`
	// Format is "json"; "text" is permitted in development only.
	Format string `koanf:"format"`
}

// Tracing configures OpenTelemetry tracing (KV-OBS-001 to KV-OBS-007).
type Tracing struct {
	// Enabled turns tracing on.
	Enabled bool `koanf:"enabled"`
	// OTLPEndpoint is the OTLP collector address.
	OTLPEndpoint string `koanf:"otlp_endpoint"`
	// Sampler is the OpenTelemetry sampler name.
	Sampler string `koanf:"sampler"`
	// SampleRatio is the fraction of traces sampled by ratio samplers.
	SampleRatio float64 `koanf:"sample_ratio"`
}

// Metrics configures Prometheus metrics.
type Metrics struct {
	// Enabled turns the metrics listener on.
	Enabled bool `koanf:"enabled"`
}

// Health configures the readiness check (KV-OBS-032).
type Health struct {
	// MaxApplyLagEntries is the largest gap between committed and applied
	// indexes at which the node still reports ready.
	MaxApplyLagEntries int `koanf:"max_apply_lag_entries"`
}
