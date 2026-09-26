package config

import "strings"

// jwtAlgorithms are the signing algorithms a token may use: asymmetric only,
// so a verifier can never be tricked into treating its public key as an HMAC
// secret, and never "none" (KV-SEC-014).
var jwtAlgorithms = []string{"RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512"}

// traceSamplers are the OpenTelemetry sampler names that may be configured.
var traceSamplers = []string{
	"always_on", "always_off", "traceidratio",
	"parentbased_always_on", "parentbased_always_off", "parentbased_traceidratio",
}

// validateNode checks the node identity and lifecycle settings.
func (c *Config) validateNode(v *violations) {
	n := c.Node
	requireValue(v, "node.id", n.ID)
	requireOneOf(v, "node.environment", n.Environment,
		EnvironmentProduction, EnvironmentStaging, EnvironmentDevelopment)
	requireValue(v, "node.data_dir", n.DataDir)
	requirePositive(v, "node.shutdown_grace", n.ShutdownGrace)
}

// validateListen checks the listen and advertise addresses. Optional
// addresses are checked only when set.
func (c *Config) validateListen(v *violations) {
	requireHostPort(v, "listen.client", c.Listen.Client)
	requireHostPort(v, "listen.peer", c.Listen.Peer)
	if c.Observability.Metrics.Enabled || c.Listen.Metrics != "" {
		requireHostPort(v, "listen.metrics", c.Listen.Metrics)
	}
	for _, a := range []setting{
		{"listen.diagnostics", c.Listen.Diagnostics},
		{"advertise.client", c.Advertise.Client},
		{"advertise.peer", c.Advertise.Peer},
	} {
		if a.value != "" {
			requireHostPort(v, a.key, a.value)
		}
	}
}

// setting pairs a key with its string value, for rules applied to several
// keys in a fixed order.
type setting struct {
	key, value string
}

// validateAuth checks authentication and authorization. With authentication
// disabled, it enforces the development-only guards (KV-SEC-041, KV-SEC-042).
func (c *Config) validateAuth(v *violations) {
	a := c.Auth
	requireOneOf(v, "auth.mode", a.Mode, AuthModeEnabled, AuthModeDisabled)
	c.validateJWT(v)
	validateScopes(v, a.Scopes)

	if a.Mode != AuthModeDisabled {
		return
	}
	if c.Node.Environment != EnvironmentDevelopment {
		v.add("auth.mode", "may be disabled only when node.environment is development")
	}
	if a.InsecureAllowRemote {
		return
	}
	for _, l := range []setting{
		{"listen.client", c.Listen.Client},
		{"listen.peer", c.Listen.Peer},
		{"listen.metrics", c.Listen.Metrics},
		{"listen.diagnostics", c.Listen.Diagnostics},
	} {
		if l.value != "" && !isLoopback(l.value) {
			v.add(l.key, "must be a loopback address while authentication is disabled, unless auth.insecure_allow_remote is true")
		}
	}
}

// validateJWT checks token verification. The issuer and key-set URLs are
// required only while authentication is enabled.
func (c *Config) validateJWT(v *violations) {
	j := c.Auth.JWT
	if c.Auth.Mode == AuthModeEnabled {
		requireHTTPSURL(v, "auth.jwt.issuer_url", j.IssuerURL)
		requireHTTPSURL(v, "auth.jwt.jwks_url", j.JWKSURL)
	}
	requireEntries(v, "auth.jwt.audience", j.Audience)
	requireEntries(v, "auth.jwt.algorithms", j.Algorithms)
	for _, alg := range j.Algorithms {
		requireOneOf(v, "auth.jwt.algorithms", alg, jwtAlgorithms...)
	}
	requireNonNegative(v, "auth.jwt.clock_skew", j.ClockSkew)
	requirePositive(v, "auth.jwt.refresh_interval", j.RefreshInterval)
	requirePositive(v, "auth.jwt.min_refresh_interval", j.MinRefreshInterval)
	requirePositive(v, "auth.jwt.http_timeout", j.HTTPTimeout)
	requirePositive(v, "auth.jwt.max_response_bytes", j.MaxResponseBytes)
	if j.MinRSABits < 2048 {
		v.add("auth.jwt.min_rsa_bits", "must be at least 2048 (KV-SEC-018)")
	}
	requireNonNegative(v, "auth.jwt.claims_cache.max_entries", j.ClaimsCache.MaxEntries)
}

// validateScopes checks that every scope is named and that no two operation
// classes share a scope, so kv:write can never imply kv:read (KV-SEC-022).
func validateScopes(v *violations, s Scopes) {
	scopes := []setting{
		{"auth.scopes.read", s.Read},
		{"auth.scopes.write", s.Write},
		{"auth.scopes.watch", s.Watch},
		{"auth.scopes.admin", s.Admin},
	}
	seen := make(map[string]string, len(scopes))
	for _, sc := range scopes {
		requireValue(v, sc.key, sc.value)
		if other, dup := seen[sc.value]; dup && sc.value != "" {
			v.addf(sc.key, "must differ from %s; one scope must never grant another", other)
		}
		seen[sc.value] = sc.key
	}
}

// validateTLS checks mutual TLS. TLS may be off only when authentication is
// off (KV-SEC-001); the peer settings are required only when peers exist.
func (c *Config) validateTLS(v *violations) {
	t := c.TLS
	requireOneOf(v, "tls.min_version", t.MinVersion, "1.3", "1.2")
	if !t.Enabled {
		if c.Auth.Mode == AuthModeEnabled {
			v.add("tls.enabled", "may be false only when auth.mode is disabled (KV-SEC-001)")
		}
		return
	}
	requireValue(v, "tls.cert_file", t.CertFile)
	requireValue(v, "tls.key_file", t.KeyFile)
	requireValue(v, "tls.client_ca_file", t.ClientCAFile)
	requireEntries(v, "tls.allowed_client_sans", t.AllowedClientSANs)
	if len(c.Cluster.InitialPeers) > 0 {
		requireValue(v, "tls.peer_ca_file", t.PeerCAFile)
		requireEntries(v, "tls.allowed_peer_sans", t.AllowedPeerSANs)
	}
}

// validateLimits checks the request and rate limits.
func (c *Config) validateLimits(v *violations) {
	l := c.Limits
	requirePositive(v, "limits.max_key_bytes", l.MaxKeyBytes)
	requirePositive(v, "limits.max_value_bytes", l.MaxValueBytes)
	requirePositive(v, "limits.max_request_bytes", l.MaxRequestBytes)
	requirePositive(v, "limits.max_concurrent_streams", l.MaxConcurrentStreams)
	requirePositive(v, "limits.max_watchers_per_principal", l.MaxWatchersPerPrincipal)
	if l.MaxKeyBytes > l.MaxRequestBytes {
		v.add("limits.max_key_bytes", "must not exceed limits.max_request_bytes")
	}
	if l.MaxValueBytes > l.MaxRequestBytes {
		v.add("limits.max_value_bytes", "must not exceed limits.max_request_bytes")
	}
	for _, b := range []struct {
		prefix string
		limit  RateLimit
	}{
		{"limits.rate_limit.read", l.RateLimit.Read},
		{"limits.rate_limit.write", l.RateLimit.Write},
		{"limits.rate_limit.admin", l.RateLimit.Admin},
	} {
		requirePositive(v, b.prefix+".rate", b.limit.Rate)
		requirePositive(v, b.prefix+".burst", b.limit.Burst)
	}
}

// validateStorage checks the storage engine and command log settings.
func (c *Config) validateStorage(v *violations) {
	s := c.Storage
	requireOneOf(v, "storage.engine", s.Engine, "pebble")
	requireOneOf(v, "storage.fsync", s.Fsync, FsyncAlways, FsyncInterval, FsyncOS)
	if s.Fsync == FsyncInterval {
		requirePositive(v, "storage.fsync_interval", s.FsyncInterval)
	}
	requirePositive(v, "storage.wal_segment_bytes", s.WALSegmentBytes)
	requireNonNegative(v, "storage.wal_retain_segments", s.WALRetainSegments)
	requirePositive(v, "storage.snapshot_entries", s.SnapshotEntries)
	requirePositive(v, "storage.snapshot_interval", s.SnapshotInterval)
	requirePositive(v, "storage.quota_bytes", s.QuotaBytes)
	requireNonNegative(v, "storage.auto_compaction_retention", s.AutoCompactionRetention)
}

// validateCluster checks membership, sharding, and Raft. Settings that a
// MUST requirement pins accept only the mandated value.
func (c *Config) validateCluster(v *violations) {
	cl := c.Cluster
	for _, peer := range cl.InitialPeers {
		name, addr, ok := strings.Cut(peer, "=")
		if !ok || name == "" {
			v.add("cluster.initial_peers", "entries must have the form name=host:port")
			continue
		}
		requireHostPort(v, "cluster.initial_peers", addr)
	}
	requirePositive(v, "cluster.slot_count", cl.SlotCount)
	requirePositive(v, "cluster.groups", cl.Groups)
	requirePositive(v, "cluster.replication_factor", cl.ReplicationFactor)

	r := cl.Raft
	requirePositive(v, "cluster.raft.tick_interval", r.TickInterval)
	requirePositive(v, "cluster.raft.heartbeat_ticks", r.HeartbeatTicks)
	if r.ElectionTicks < 10*r.HeartbeatTicks {
		v.add("cluster.raft.election_ticks", "must be at least 10 × cluster.raft.heartbeat_ticks (KV-CON-004)")
	}
	requirePositive(v, "cluster.raft.max_inflight_msgs", r.MaxInflightMsgs)
	requirePositive(v, "cluster.raft.max_size_per_msg", r.MaxSizePerMsg)
	if !r.PreVote {
		v.add("cluster.raft.pre_vote", "must be true (KV-CON-005)")
	}
	if !r.CheckQuorum {
		v.add("cluster.raft.check_quorum", "must be true (KV-CON-006)")
	}

	requireOneOf(v, "cluster.read.default_consistency", cl.Read.DefaultConsistency, "linearizable")
	requirePositive(v, "cluster.read.leader_wait_timeout", cl.Read.LeaderWaitTimeout)
}

// validateObservability checks logging, tracing, and health settings.
func (c *Config) validateObservability(v *violations) {
	o := c.Observability
	requireOneOf(v, "observability.log.level", o.Log.Level, "debug", "info", "warn", "error")
	requireOneOf(v, "observability.log.format", o.Log.Format, "json", "text")
	if o.Log.Format == "text" && c.Node.Environment != EnvironmentDevelopment {
		v.add("observability.log.format", "may be text only when node.environment is development (KV-OBS-020)")
	}
	if o.Tracing.Enabled {
		requireValue(v, "observability.tracing.otlp_endpoint", o.Tracing.OTLPEndpoint)
	}
	requireOneOf(v, "observability.tracing.sampler", o.Tracing.Sampler, traceSamplers...)
	if o.Tracing.SampleRatio < 0 || o.Tracing.SampleRatio > 1 {
		v.add("observability.tracing.sample_ratio", "must be between 0 and 1")
	}
	requirePositive(v, "observability.health.max_apply_lag_entries", o.Health.MaxApplyLagEntries)
}

// requireEntries records a violation when list is empty or has an empty
// entry.
func requireEntries(v *violations, key string, list []string) {
	if len(list) == 0 {
		v.add(key, "must have at least one entry")
		return
	}
	for _, item := range list {
		if strings.TrimSpace(item) == "" {
			v.add(key, "must not contain empty entries")
			return
		}
	}
}
