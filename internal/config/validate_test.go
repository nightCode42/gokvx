package config_test

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/config"
)

// validConfig returns the defaults plus the deployment-specific values that
// have no default, which together form a valid configuration.
func validConfig() config.Config {
	c := config.Defaults()
	c.Auth.JWT.IssuerURL = "https://issuer.test/"
	c.Auth.JWT.JWKSURL = "https://issuer.test/.well-known/jwks.json"
	c.TLS.AllowedClientSANs = []string{"svc-microservice-1"}
	return c
}

// devWithoutAuth returns a valid development configuration with
// authentication and TLS disabled, listening on loopback only.
func devWithoutAuth() config.Config {
	c := validConfig()
	c.Node.Environment = config.EnvironmentDevelopment
	c.Auth.Mode = config.AuthModeDisabled
	c.TLS.Enabled = false
	c.Listen.Client = "127.0.0.1:2379"
	c.Listen.Peer = "localhost:2380"
	c.Listen.Diagnostics = "[::1]:6060"
	return c
}

// TestValidConfigPasses checks the fixtures the rule tests start from.
func TestValidConfigPasses(t *testing.T) {
	t.Parallel()

	for name, cfg := range map[string]config.Config{"valid": validConfig(), "development": devWithoutAuth()} {
		assert.NoError(t, cfg.Validate(), name)
	}
}

// TestDefaultsRequireOnlyDeploymentValues checks that the built-in defaults
// are complete except for the values no default can know, which are trusted
// only once configured.
func TestDefaultsRequireOnlyDeploymentValues(t *testing.T) {
	t.Parallel()

	defaults := config.Defaults()

	assert.Equal(t,
		[]string{"auth.jwt.issuer_url", "auth.jwt.jwks_url", "tls.allowed_client_sans"},
		violationFields(t, defaults.Validate()))
}

// ruleCase is one validation rule: a change to a valid configuration and the
// key the resulting violation must name. An empty wantKey means the change
// must be accepted.
type ruleCase struct {
	name    string
	base    func() config.Config
	mutate  func(c *config.Config)
	wantKey string
}

// runRuleCases applies each case to its base configuration and checks the
// outcome.
func runRuleCases(t *testing.T, cases []ruleCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := validConfig
			if tc.base != nil {
				base = tc.base
			}
			cfg := base()
			tc.mutate(&cfg)

			err := cfg.Validate()
			if tc.wantKey == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, violationFields(t, err), tc.wantKey)
		})
	}
}

// TestValidateNodeAndListeners_KV_CFG_002 covers the node and address rules.
// Verifies: KV-CFG-002.
func TestValidateNodeAndListeners_KV_CFG_002(t *testing.T) {
	t.Parallel()

	runRuleCases(t, []ruleCase{
		{name: "empty node id", mutate: func(c *config.Config) { c.Node.ID = "" }, wantKey: "node.id"},
		{name: "unknown environment", mutate: func(c *config.Config) { c.Node.Environment = "prod" }, wantKey: "node.environment"},
		{name: "empty data dir", mutate: func(c *config.Config) { c.Node.DataDir = "" }, wantKey: "node.data_dir"},
		{name: "zero shutdown grace", mutate: func(c *config.Config) { c.Node.ShutdownGrace = 0 }, wantKey: "node.shutdown_grace"},
		{name: "client address without port", mutate: func(c *config.Config) { c.Listen.Client = "0.0.0.0" }, wantKey: "listen.client"},
		{name: "peer port zero", mutate: func(c *config.Config) { c.Listen.Peer = ":0" }, wantKey: "listen.peer"},
		{name: "port out of range", mutate: func(c *config.Config) { c.Listen.Peer = ":70000" }, wantKey: "listen.peer"},
		{name: "metrics required when enabled", mutate: func(c *config.Config) { c.Listen.Metrics = "" }, wantKey: "listen.metrics"},
		{
			name:   "metrics optional when disabled",
			mutate: func(c *config.Config) { c.Listen.Metrics = ""; c.Observability.Metrics.Enabled = false },
		},
		{name: "bad diagnostics address", mutate: func(c *config.Config) { c.Listen.Diagnostics = "pprof" }, wantKey: "listen.diagnostics"},
		{name: "diagnostics disabled", mutate: func(c *config.Config) { c.Listen.Diagnostics = "" }},
		{name: "bad advertise client", mutate: func(c *config.Config) { c.Advertise.Client = "gokvx-0" }, wantKey: "advertise.client"},
		{name: "bad advertise peer", mutate: func(c *config.Config) { c.Advertise.Peer = "gokvx-0" }, wantKey: "advertise.peer"},
		{name: "advertise set", mutate: func(c *config.Config) { c.Advertise.Client = "gokvx-0.gokvx:2379" }},
	})
}

// TestValidateAuth_KV_SEC_014 covers the token verification and scope rules.
// Verifies: KV-CFG-002, KV-SEC-014, KV-SEC-018, KV-SEC-022.
func TestValidateAuth_KV_SEC_014(t *testing.T) {
	t.Parallel()

	runRuleCases(t, []ruleCase{
		{name: "unknown auth mode", mutate: func(c *config.Config) { c.Auth.Mode = "off" }, wantKey: "auth.mode"},
		{name: "issuer required", mutate: func(c *config.Config) { c.Auth.JWT.IssuerURL = "" }, wantKey: "auth.jwt.issuer_url"},
		{name: "issuer must be https", mutate: func(c *config.Config) { c.Auth.JWT.IssuerURL = "http://issuer.test/" }, wantKey: "auth.jwt.issuer_url"},
		{name: "jwks must be absolute", mutate: func(c *config.Config) { c.Auth.JWT.JWKSURL = "/jwks.json" }, wantKey: "auth.jwt.jwks_url"},
		{name: "audience required", mutate: func(c *config.Config) { c.Auth.JWT.Audience = nil }, wantKey: "auth.jwt.audience"},
		{name: "empty audience entry", mutate: func(c *config.Config) { c.Auth.JWT.Audience = []string{" "} }, wantKey: "auth.jwt.audience"},
		{name: "algorithms required", mutate: func(c *config.Config) { c.Auth.JWT.Algorithms = nil }, wantKey: "auth.jwt.algorithms"},
		{name: "algorithm none", mutate: func(c *config.Config) { c.Auth.JWT.Algorithms = []string{"none"} }, wantKey: "auth.jwt.algorithms"},
		{name: "symmetric algorithm", mutate: func(c *config.Config) { c.Auth.JWT.Algorithms = []string{"HS256"} }, wantKey: "auth.jwt.algorithms"},
		{name: "asymmetric algorithms", mutate: func(c *config.Config) { c.Auth.JWT.Algorithms = []string{"ES256", "PS512"} }},
		{name: "negative clock skew", mutate: func(c *config.Config) { c.Auth.JWT.ClockSkew = -time.Second }, wantKey: "auth.jwt.clock_skew"},
		{name: "zero clock skew", mutate: func(c *config.Config) { c.Auth.JWT.ClockSkew = 0 }},
		{name: "zero refresh", mutate: func(c *config.Config) { c.Auth.JWT.RefreshInterval = 0 }, wantKey: "auth.jwt.refresh_interval"},
		{name: "zero min refresh", mutate: func(c *config.Config) { c.Auth.JWT.MinRefreshInterval = 0 }, wantKey: "auth.jwt.min_refresh_interval"},
		{name: "zero http timeout", mutate: func(c *config.Config) { c.Auth.JWT.HTTPTimeout = 0 }, wantKey: "auth.jwt.http_timeout"},
		{name: "zero response limit", mutate: func(c *config.Config) { c.Auth.JWT.MaxResponseBytes = 0 }, wantKey: "auth.jwt.max_response_bytes"},
		{name: "weak rsa", mutate: func(c *config.Config) { c.Auth.JWT.MinRSABits = 1024 }, wantKey: "auth.jwt.min_rsa_bits"},
		{name: "negative cache", mutate: func(c *config.Config) { c.Auth.JWT.ClaimsCache.MaxEntries = -1 }, wantKey: "auth.jwt.claims_cache.max_entries"},
		{name: "cache disabled", mutate: func(c *config.Config) { c.Auth.JWT.ClaimsCache.MaxEntries = 0 }},
		{name: "empty scope", mutate: func(c *config.Config) { c.Auth.Scopes.Watch = "" }, wantKey: "auth.scopes.watch"},
		{name: "write grants read", mutate: func(c *config.Config) { c.Auth.Scopes.Write = "kv:read" }, wantKey: "auth.scopes.write"},
	})
}

// TestValidateDevelopmentMode_KV_SEC_041 covers the guards that must all be
// set before an unauthenticated node is reachable.
// Verifies: KV-SEC-001, KV-SEC-041, KV-SEC-042.
func TestValidateDevelopmentMode_KV_SEC_041(t *testing.T) {
	t.Parallel()

	runRuleCases(t, []ruleCase{
		{
			name:    "disabled outside development",
			base:    devWithoutAuth,
			mutate:  func(c *config.Config) { c.Node.Environment = config.EnvironmentStaging },
			wantKey: "auth.mode",
		},
		{
			name:    "disabled on all interfaces",
			base:    devWithoutAuth,
			mutate:  func(c *config.Config) { c.Listen.Client = "0.0.0.0:2379" },
			wantKey: "listen.client",
		},
		{
			name:    "disabled with a malformed address",
			base:    devWithoutAuth,
			mutate:  func(c *config.Config) { c.Listen.Peer = "localhost" },
			wantKey: "listen.peer",
		},
		{
			name:    "disabled with metrics on the network",
			base:    devWithoutAuth,
			mutate:  func(c *config.Config) { c.Listen.Metrics = ":9100" },
			wantKey: "listen.metrics",
		},
		{
			name: "disabled and remote, explicitly allowed",
			base: devWithoutAuth,
			mutate: func(c *config.Config) {
				c.Listen.Client = "0.0.0.0:2379"
				c.Auth.InsecureAllowRemote = true
			},
		},
		{
			name:   "issuer not required while disabled",
			base:   devWithoutAuth,
			mutate: func(c *config.Config) { c.Auth.JWT.IssuerURL = ""; c.Auth.JWT.JWKSURL = "" },
		},
		{name: "tls off while auth on", mutate: func(c *config.Config) { c.TLS.Enabled = false }, wantKey: "tls.enabled"},
	})
}

// TestValidateTLS_KV_SEC_002 covers the certificate settings.
// Verifies: KV-CFG-002, KV-SEC-002, KV-SEC-004.
func TestValidateTLS_KV_SEC_002(t *testing.T) {
	t.Parallel()

	withPeers := func(c *config.Config) { c.Cluster.InitialPeers = []string{"gokvx-1=gokvx-1:2380"} }

	runRuleCases(t, []ruleCase{
		{name: "tls 1.1", mutate: func(c *config.Config) { c.TLS.MinVersion = "1.1" }, wantKey: "tls.min_version"},
		{name: "tls 1.2 explicitly", mutate: func(c *config.Config) { c.TLS.MinVersion = "1.2" }},
		{name: "cert required", mutate: func(c *config.Config) { c.TLS.CertFile = "" }, wantKey: "tls.cert_file"},
		{name: "key required", mutate: func(c *config.Config) { c.TLS.KeyFile = "" }, wantKey: "tls.key_file"},
		{name: "client ca required", mutate: func(c *config.Config) { c.TLS.ClientCAFile = "" }, wantKey: "tls.client_ca_file"},
		{name: "client sans required", mutate: func(c *config.Config) { c.TLS.AllowedClientSANs = nil }, wantKey: "tls.allowed_client_sans"},
		{name: "peer sans required with peers", mutate: withPeers, wantKey: "tls.allowed_peer_sans"},
		{
			name:    "peer ca required with peers",
			mutate:  func(c *config.Config) { withPeers(c); c.TLS.PeerCAFile = "" },
			wantKey: "tls.peer_ca_file",
		},
		{
			name:   "peers with peer trust",
			mutate: func(c *config.Config) { withPeers(c); c.TLS.AllowedPeerSANs = []string{"gokvx-*.gokvx"} },
		},
	})
}

// TestValidateLimitsAndStorage_KV_CFG_002 covers the limit and storage rules.
// Verifies: KV-CFG-002, KV-STO-005.
func TestValidateLimitsAndStorage_KV_CFG_002(t *testing.T) {
	t.Parallel()

	runRuleCases(t, []ruleCase{
		{name: "zero key limit", mutate: func(c *config.Config) { c.Limits.MaxKeyBytes = 0 }, wantKey: "limits.max_key_bytes"},
		{name: "zero value limit", mutate: func(c *config.Config) { c.Limits.MaxValueBytes = 0 }, wantKey: "limits.max_value_bytes"},
		{name: "zero request limit", mutate: func(c *config.Config) { c.Limits.MaxRequestBytes = 0 }, wantKey: "limits.max_request_bytes"},
		{name: "zero streams", mutate: func(c *config.Config) { c.Limits.MaxConcurrentStreams = 0 }, wantKey: "limits.max_concurrent_streams"},
		{name: "zero watchers", mutate: func(c *config.Config) { c.Limits.MaxWatchersPerPrincipal = 0 }, wantKey: "limits.max_watchers_per_principal"},
		{name: "key above request", mutate: func(c *config.Config) { c.Limits.MaxKeyBytes = 5 << 20 }, wantKey: "limits.max_key_bytes"},
		{name: "value above request", mutate: func(c *config.Config) { c.Limits.MaxValueBytes = 5 << 20 }, wantKey: "limits.max_value_bytes"},
		{name: "zero read rate", mutate: func(c *config.Config) { c.Limits.RateLimit.Read.Rate = 0 }, wantKey: "limits.rate_limit.read.rate"},
		{name: "zero write burst", mutate: func(c *config.Config) { c.Limits.RateLimit.Write.Burst = 0 }, wantKey: "limits.rate_limit.write.burst"},
		{name: "zero admin rate", mutate: func(c *config.Config) { c.Limits.RateLimit.Admin.Rate = 0 }, wantKey: "limits.rate_limit.admin.rate"},
		{name: "unknown engine", mutate: func(c *config.Config) { c.Storage.Engine = "bolt" }, wantKey: "storage.engine"},
		{name: "unknown fsync", mutate: func(c *config.Config) { c.Storage.Fsync = "never" }, wantKey: "storage.fsync"},
		{
			name:    "interval fsync without interval",
			mutate:  func(c *config.Config) { c.Storage.Fsync = config.FsyncInterval; c.Storage.FsyncInterval = 0 },
			wantKey: "storage.fsync_interval",
		},
		{name: "os fsync", mutate: func(c *config.Config) { c.Storage.Fsync = config.FsyncOS; c.Storage.FsyncInterval = 0 }},
		{name: "zero segment", mutate: func(c *config.Config) { c.Storage.WALSegmentBytes = 0 }, wantKey: "storage.wal_segment_bytes"},
		{name: "negative retention", mutate: func(c *config.Config) { c.Storage.WALRetainSegments = -1 }, wantKey: "storage.wal_retain_segments"},
		{name: "zero snapshot entries", mutate: func(c *config.Config) { c.Storage.SnapshotEntries = 0 }, wantKey: "storage.snapshot_entries"},
		{name: "zero snapshot interval", mutate: func(c *config.Config) { c.Storage.SnapshotInterval = 0 }, wantKey: "storage.snapshot_interval"},
		{name: "zero quota", mutate: func(c *config.Config) { c.Storage.QuotaBytes = 0 }, wantKey: "storage.quota_bytes"},
		{
			name:    "negative compaction retention",
			mutate:  func(c *config.Config) { c.Storage.AutoCompactionRetention = -1 },
			wantKey: "storage.auto_compaction_retention",
		},
		{name: "compaction disabled", mutate: func(c *config.Config) { c.Storage.AutoCompactionRetention = 0 }},
	})
}

// TestValidateCluster_KV_CON_004 covers the cluster rules, including the
// settings pinned by MUST requirements.
// Verifies: KV-CFG-002, KV-CON-004, KV-CON-005, KV-CON-006, KV-API-080.
func TestValidateCluster_KV_CON_004(t *testing.T) {
	t.Parallel()

	runRuleCases(t, []ruleCase{
		{name: "peer without name", mutate: func(c *config.Config) { c.Cluster.InitialPeers = []string{"gokvx-1:2380"} }, wantKey: "cluster.initial_peers"},
		{name: "peer without port", mutate: func(c *config.Config) { c.Cluster.InitialPeers = []string{"n=gokvx-1"} }, wantKey: "cluster.initial_peers"},
		{name: "zero slots", mutate: func(c *config.Config) { c.Cluster.SlotCount = 0 }, wantKey: "cluster.slot_count"},
		{name: "zero groups", mutate: func(c *config.Config) { c.Cluster.Groups = 0 }, wantKey: "cluster.groups"},
		{name: "zero replicas", mutate: func(c *config.Config) { c.Cluster.ReplicationFactor = 0 }, wantKey: "cluster.replication_factor"},
		{name: "zero tick", mutate: func(c *config.Config) { c.Cluster.Raft.TickInterval = 0 }, wantKey: "cluster.raft.tick_interval"},
		{name: "zero heartbeat", mutate: func(c *config.Config) { c.Cluster.Raft.HeartbeatTicks = 0 }, wantKey: "cluster.raft.heartbeat_ticks"},
		{name: "election too short", mutate: func(c *config.Config) { c.Cluster.Raft.ElectionTicks = 9 }, wantKey: "cluster.raft.election_ticks"},
		{
			name:    "election scales with heartbeat",
			mutate:  func(c *config.Config) { c.Cluster.Raft.HeartbeatTicks = 2; c.Cluster.Raft.ElectionTicks = 19 },
			wantKey: "cluster.raft.election_ticks",
		},
		{name: "zero inflight", mutate: func(c *config.Config) { c.Cluster.Raft.MaxInflightMsgs = 0 }, wantKey: "cluster.raft.max_inflight_msgs"},
		{name: "zero message size", mutate: func(c *config.Config) { c.Cluster.Raft.MaxSizePerMsg = 0 }, wantKey: "cluster.raft.max_size_per_msg"},
		{name: "prevote off", mutate: func(c *config.Config) { c.Cluster.Raft.PreVote = false }, wantKey: "cluster.raft.pre_vote"},
		{name: "checkquorum off", mutate: func(c *config.Config) { c.Cluster.Raft.CheckQuorum = false }, wantKey: "cluster.raft.check_quorum"},
		{
			name:    "cheaper default consistency",
			mutate:  func(c *config.Config) { c.Cluster.Read.DefaultConsistency = "serializable" },
			wantKey: "cluster.read.default_consistency",
		},
		{name: "zero leader wait", mutate: func(c *config.Config) { c.Cluster.Read.LeaderWaitTimeout = 0 }, wantKey: "cluster.read.leader_wait_timeout"},
	})
}

// TestValidateObservability_KV_CFG_002 covers the logging, tracing, and
// health rules.
// Verifies: KV-CFG-002, KV-OBS-020.
func TestValidateObservability_KV_CFG_002(t *testing.T) {
	t.Parallel()

	runRuleCases(t, []ruleCase{
		{name: "unknown level", mutate: func(c *config.Config) { c.Observability.Log.Level = "trace" }, wantKey: "observability.log.level"},
		{name: "unknown format", mutate: func(c *config.Config) { c.Observability.Log.Format = "xml" }, wantKey: "observability.log.format"},
		{name: "text outside development", mutate: func(c *config.Config) { c.Observability.Log.Format = "text" }, wantKey: "observability.log.format"},
		{name: "text in development", base: devWithoutAuth, mutate: func(c *config.Config) { c.Observability.Log.Format = "text" }},
		{
			name:    "tracing without endpoint",
			mutate:  func(c *config.Config) { c.Observability.Tracing.OTLPEndpoint = "" },
			wantKey: "observability.tracing.otlp_endpoint",
		},
		{
			name: "tracing disabled without endpoint",
			mutate: func(c *config.Config) {
				c.Observability.Tracing.Enabled = false
				c.Observability.Tracing.OTLPEndpoint = ""
			},
		},
		{name: "unknown sampler", mutate: func(c *config.Config) { c.Observability.Tracing.Sampler = "sometimes" }, wantKey: "observability.tracing.sampler"},
		{name: "ratio above one", mutate: func(c *config.Config) { c.Observability.Tracing.SampleRatio = 1.5 }, wantKey: "observability.tracing.sample_ratio"},
		{name: "negative ratio", mutate: func(c *config.Config) { c.Observability.Tracing.SampleRatio = -0.1 }, wantKey: "observability.tracing.sample_ratio"},
		{
			name:    "zero apply lag",
			mutate:  func(c *config.Config) { c.Observability.Health.MaxApplyLagEntries = 0 },
			wantKey: "observability.health.max_apply_lag_entries",
		},
	})
}

// TestValidateReportsEveryViolationAtOnce_KV_CFG_002 checks that validation
// does not stop at the first problem, and that violations are sorted by key.
// Verifies: KV-CFG-002.
func TestValidateReportsEveryViolationAtOnce_KV_CFG_002(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Storage.Fsync = "never"
	cfg.Node.ID = ""
	cfg.Cluster.Raft.PreVote = false

	fields := violationFields(t, cfg.Validate())

	assert.Equal(t, []string{"cluster.raft.pre_vote", "node.id", "storage.fsync"}, fields)
	assert.True(t, slices.IsSorted(fields))
}
