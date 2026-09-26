package config

// TLS configures mutual TLS for client and peer traffic (KV-SEC-001 to
// KV-SEC-007).
type TLS struct {
	// Enabled turns TLS on; it may be off only when authentication is off.
	Enabled bool `koanf:"enabled"`
	// MinVersion is the lowest accepted protocol version: "1.3" or "1.2".
	MinVersion string `koanf:"min_version"`
	// CertFile is the path of the node's certificate.
	CertFile string `koanf:"cert_file"`
	// KeyFile is the path of the node's private key.
	KeyFile string `koanf:"key_file"`
	// ClientCAFile is the path of the CA bundle trusted for clients.
	ClientCAFile string `koanf:"client_ca_file"`
	// PeerCAFile is the path of the CA bundle trusted for peers.
	PeerCAFile string `koanf:"peer_ca_file"`
	// Reload hot-swaps certificates when the files change (KV-SEC-005).
	Reload bool `koanf:"reload"`
	// AllowedPeerSANs lists the certificate SANs, exact or patterns,
	// accepted from peers (KV-SEC-004).
	AllowedPeerSANs []string `koanf:"allowed_peer_sans"`
	// AllowedClientSANs lists the certificate SANs, exact or patterns,
	// accepted from clients.
	AllowedClientSANs []string `koanf:"allowed_client_sans"`
}
