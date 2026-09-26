package config

import "time"

// AuthMode selects whether requests are authenticated.
type AuthMode string

// The authentication modes.
const (
	// AuthModeEnabled requires mutual TLS and a valid token on every request.
	AuthModeEnabled AuthMode = "enabled"
	// AuthModeDisabled turns off mutual TLS and token verification; it is
	// permitted only in development (KV-SEC-040, KV-SEC-041).
	AuthModeDisabled AuthMode = "disabled"
)

// Auth configures authentication and authorization (spec §11).
type Auth struct {
	// Mode enables or disables authentication.
	Mode AuthMode `koanf:"mode"`
	// InsecureAllowRemote lets a node with authentication disabled listen on
	// non-loopback addresses (KV-SEC-042).
	InsecureAllowRemote bool `koanf:"insecure_allow_remote"`
	// JWT configures bearer-token verification.
	JWT JWT `koanf:"jwt"`
	// Scopes names the scope each operation class requires.
	Scopes Scopes `koanf:"scopes"`
}

// JWT configures how bearer tokens are verified (KV-SEC-010 to KV-SEC-019).
type JWT struct {
	// IssuerURL is the expected iss claim; an HTTPS URL.
	IssuerURL string `koanf:"issuer_url"`
	// JWKSURL is the HTTPS URL of the issuer's signing keys.
	JWKSURL string `koanf:"jwks_url"`
	// Audience lists the accepted aud claim values.
	Audience []string `koanf:"audience"`
	// Algorithms lists the accepted signing algorithms (KV-SEC-014).
	Algorithms []string `koanf:"algorithms"`
	// ClockSkew is the tolerance applied to exp, nbf, and iat.
	ClockSkew time.Duration `koanf:"clock_skew"`
	// RefreshInterval is how often the key set is refreshed in the
	// background (KV-SEC-012).
	RefreshInterval time.Duration `koanf:"refresh_interval"`
	// MinRefreshInterval rate-limits refreshes triggered by an unknown key ID
	// (KV-SEC-013).
	MinRefreshInterval time.Duration `koanf:"min_refresh_interval"`
	// HTTPTimeout bounds each key-set request (KV-SEC-018).
	HTTPTimeout time.Duration `koanf:"http_timeout"`
	// MaxResponseBytes bounds the size of a key-set response.
	MaxResponseBytes int64 `koanf:"max_response_bytes"`
	// MinRSABits is the smallest accepted RSA modulus.
	MinRSABits int `koanf:"min_rsa_bits"`
	// ClaimsCache configures the cache of verified tokens (KV-SEC-017).
	ClaimsCache ClaimsCache `koanf:"claims_cache"`
}

// ClaimsCache configures the cache of verified token claims.
type ClaimsCache struct {
	// MaxEntries bounds the cache; zero disables caching.
	MaxEntries int `koanf:"max_entries"`
}

// Scopes names the OAuth scope each operation class requires (KV-SEC-022).
type Scopes struct {
	// Read is required by Get and List.
	Read string `koanf:"read"`
	// Write is required by mutations and lease operations.
	Write string `koanf:"write"`
	// Watch is required by Watch.
	Watch string `koanf:"watch"`
	// Admin is required by compaction, membership, and slot moves.
	Admin string `koanf:"admin"`
}
