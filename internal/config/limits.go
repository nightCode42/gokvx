package config

// Limits bounds what a single request or principal can consume (KV-DAT-011,
// KV-DAT-013, KV-SEC-032, KV-SEC-033).
type Limits struct {
	// MaxKeyBytes is the longest accepted key.
	MaxKeyBytes int `koanf:"max_key_bytes"`
	// MaxValueBytes is the largest accepted value.
	MaxValueBytes int `koanf:"max_value_bytes"`
	// MaxRequestBytes is the largest accepted request message.
	MaxRequestBytes int `koanf:"max_request_bytes"`
	// MaxConcurrentStreams bounds concurrent streams per connection.
	MaxConcurrentStreams int `koanf:"max_concurrent_streams"`
	// MaxWatchersPerPrincipal bounds the open watches of one principal.
	MaxWatchersPerPrincipal int `koanf:"max_watchers_per_principal"`
	// RateLimit configures per-principal token buckets by operation class.
	RateLimit RateLimits `koanf:"rate_limit"`
}

// RateLimits holds one token bucket per operation class.
type RateLimits struct {
	// Read limits Get, List, and Watch.
	Read RateLimit `koanf:"read"`
	// Write limits mutations.
	Write RateLimit `koanf:"write"`
	// Admin limits administrative operations.
	Admin RateLimit `koanf:"admin"`
}

// RateLimit is one token bucket.
type RateLimit struct {
	// Rate is the sustained rate, in requests per second.
	Rate float64 `koanf:"rate"`
	// Burst is the largest number of requests admitted at once.
	Burst int `koanf:"burst"`
}
