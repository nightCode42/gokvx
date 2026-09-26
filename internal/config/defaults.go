package config

import (
	"time"

	"github.com/nightCode42/gokvx/internal/shard"
)

// Defaults returns the built-in configuration: every default stated in spec
// Appendix D (KV-CFG-020).
//
// Appendix D marks some values as deployment-specific examples: the advertise
// addresses, the issuer and key-set URLs, the initial peers, and the
// certificate SAN allowlists. Those default to empty, so nothing is trusted
// until it is configured, and validation requires them where they are needed.
func Defaults() Config {
	return Config{
		Node:          defaultNode(),
		Listen:        defaultListen(),
		Advertise:     Advertise{},
		Auth:          defaultAuth(),
		TLS:           defaultTLS(),
		Limits:        defaultLimits(),
		Storage:       defaultStorage(),
		Cluster:       defaultCluster(),
		Observability: defaultObservability(),
	}
}

// defaultNode returns the default node identity and lifecycle settings.
func defaultNode() Node {
	return Node{
		ID:            "gokvx-0",
		Environment:   EnvironmentProduction,
		DataDir:       "/var/lib/gokvx",
		ShutdownGrace: 20 * time.Second,
	}
}

// defaultListen returns the default listener addresses.
func defaultListen() Listen {
	return Listen{
		Client:      "0.0.0.0:2379",
		Peer:        "0.0.0.0:2380",
		Metrics:     "127.0.0.1:9100",
		Diagnostics: "127.0.0.1:6060",
	}
}

// defaultAuth returns the default authentication settings: enabled, with no
// trusted issuer until one is configured.
func defaultAuth() Auth {
	return Auth{
		Mode:                AuthModeEnabled,
		InsecureAllowRemote: false,
		JWT: JWT{
			Audience:           []string{"gokvx"},
			Algorithms:         []string{"RS256"},
			ClockSkew:          60 * time.Second,
			RefreshInterval:    15 * time.Minute,
			MinRefreshInterval: 60 * time.Second,
			HTTPTimeout:        5 * time.Second,
			MaxResponseBytes:   1 << 20,
			MinRSABits:         2048,
			ClaimsCache:        ClaimsCache{MaxEntries: 10000},
		},
		Scopes: Scopes{
			Read:  "kv:read",
			Write: "kv:write",
			Watch: "kv:watch",
			Admin: "kv:admin",
		},
	}
}

// defaultTLS returns the default TLS settings: enabled, TLS 1.3, with no
// trusted SANs until they are configured.
func defaultTLS() TLS {
	return TLS{
		Enabled:      true,
		MinVersion:   "1.3",
		CertFile:     "/etc/gokvx/tls/tls.crt",
		KeyFile:      "/etc/gokvx/tls/tls.key",
		ClientCAFile: "/etc/gokvx/tls/client-ca.crt",
		PeerCAFile:   "/etc/gokvx/tls/peer-ca.crt",
		Reload:       true,
	}
}

// defaultLimits returns the default request and rate limits.
func defaultLimits() Limits {
	return Limits{
		MaxKeyBytes:             1024,
		MaxValueBytes:           1 << 20,
		MaxRequestBytes:         4 << 20,
		MaxConcurrentStreams:    1000,
		MaxWatchersPerPrincipal: 100,
		RateLimit: RateLimits{
			Read:  RateLimit{Rate: 5000, Burst: 10000},
			Write: RateLimit{Rate: 1000, Burst: 2000},
			Admin: RateLimit{Rate: 10, Burst: 20},
		},
	}
}

// defaultStorage returns the default storage settings, with fsync on every
// write.
func defaultStorage() Storage {
	return Storage{
		Engine:                  "pebble",
		Fsync:                   FsyncAlways,
		FsyncInterval:           100 * time.Millisecond,
		WALSegmentBytes:         64 << 20,
		WALRetainSegments:       4,
		SnapshotEntries:         10000,
		SnapshotInterval:        30 * time.Minute,
		QuotaBytes:              8 << 30,
		AutoCompactionRetention: 1000,
	}
}

// defaultCluster returns the default cluster, Raft, and read settings.
func defaultCluster() Cluster {
	return Cluster{
		Bootstrap:         false,
		SlotCount:         shard.DefaultSlotCount,
		Groups:            1,
		ReplicationFactor: 3,
		Raft: Raft{
			TickInterval:             100 * time.Millisecond,
			HeartbeatTicks:           1,
			ElectionTicks:            10,
			MaxInflightMsgs:          256,
			MaxSizePerMsg:            1 << 20,
			PreVote:                  true,
			CheckQuorum:              true,
			LeaderTransferOnShutdown: true,
		},
		Read: Read{
			DefaultConsistency:    "linearizable",
			ForwardWritesToLeader: true,
			LeaderWaitTimeout:     3 * time.Second,
		},
	}
}

// defaultObservability returns the default logging, tracing, metrics, and
// health settings.
func defaultObservability() Observability {
	return Observability{
		Log: Log{Level: "info", Format: "json"},
		Tracing: Tracing{
			Enabled:      true,
			OTLPEndpoint: "otel-collector.observability.svc:4317",
			Sampler:      "parentbased_traceidratio",
			SampleRatio:  0.05,
		},
		Metrics: Metrics{Enabled: true},
		Health:  Health{MaxApplyLagEntries: 1000},
	}
}
