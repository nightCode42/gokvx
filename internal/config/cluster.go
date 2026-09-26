package config

import "time"

// Cluster configures membership, sharding, and Raft (spec §10).
type Cluster struct {
	// Bootstrap creates a new cluster instead of joining an existing one.
	Bootstrap bool `koanf:"bootstrap"`
	// InitialPeers lists the founding members as "name=host:port".
	InitialPeers []string `koanf:"initial_peers"`
	// SlotCount is the size of the slot space; immutable after bootstrap
	// (KV-DAT-020).
	SlotCount uint32 `koanf:"slot_count"`
	// Groups is the number of shard groups created at bootstrap.
	Groups int `koanf:"groups"`
	// ReplicationFactor is the number of replicas per shard group
	// (KV-CON-014).
	ReplicationFactor int `koanf:"replication_factor"`
	// Raft configures the consensus timing and safety features.
	Raft Raft `koanf:"raft"`
	// Read configures how reads and forwarded writes are served.
	Read Read `koanf:"read"`
}

// Raft configures consensus timing and safety (KV-CON-004 to KV-CON-013).
type Raft struct {
	// TickInterval is the length of one Raft tick.
	TickInterval time.Duration `koanf:"tick_interval"`
	// HeartbeatTicks is the heartbeat interval, in ticks.
	HeartbeatTicks int `koanf:"heartbeat_ticks"`
	// ElectionTicks is the election timeout, in ticks; at least ten
	// heartbeats.
	ElectionTicks int `koanf:"election_ticks"`
	// MaxInflightMsgs bounds unacknowledged append messages per peer.
	MaxInflightMsgs int `koanf:"max_inflight_msgs"`
	// MaxSizePerMsg bounds the size of one append message.
	MaxSizePerMsg int64 `koanf:"max_size_per_msg"`
	// PreVote must be true (KV-CON-005).
	PreVote bool `koanf:"pre_vote"`
	// CheckQuorum must be true (KV-CON-006).
	CheckQuorum bool `koanf:"check_quorum"`
	// LeaderTransferOnShutdown hands leadership over before a leader exits
	// (KV-CON-013).
	LeaderTransferOnShutdown bool `koanf:"leader_transfer_on_shutdown"`
}

// Read configures how reads and forwarded writes are served.
type Read struct {
	// DefaultConsistency is the mode for reads that specify none; it must be
	// "linearizable" (KV-API-080).
	DefaultConsistency string `koanf:"default_consistency"`
	// ForwardWritesToLeader forwards writes received by a follower instead
	// of rejecting them (KV-CON-008).
	ForwardWritesToLeader bool `koanf:"forward_writes_to_leader"`
	// LeaderWaitTimeout bounds how long a request waits for a leader
	// (KV-CON-009).
	LeaderWaitTimeout time.Duration `koanf:"leader_wait_timeout"`
}
