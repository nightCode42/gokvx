package config

import "time"

// Config is the complete configuration of a gokvx node. Obtain one from
// [Load] or [Defaults], and check it with [Config.Validate] before use.
type Config struct {
	// Node identifies the node and its lifecycle.
	Node Node `koanf:"node"`
	// Listen holds the addresses the node binds to.
	Listen Listen `koanf:"listen"`
	// Advertise holds the addresses other nodes and clients use to reach it.
	Advertise Advertise `koanf:"advertise"`
	// Auth configures token authentication and authorization.
	Auth Auth `koanf:"auth"`
	// TLS configures mutual TLS for client and peer traffic.
	TLS TLS `koanf:"tls"`
	// Limits bounds what a single request or principal can consume.
	Limits Limits `koanf:"limits"`
	// Storage configures the storage engine and command log.
	Storage Storage `koanf:"storage"`
	// Cluster configures membership, sharding, and Raft.
	Cluster Cluster `koanf:"cluster"`
	// Observability configures logs, traces, metrics, and health.
	Observability Observability `koanf:"observability"`
}

// Environment names the kind of deployment a node runs in. It decides which
// settings are permitted, never where settings come from.
type Environment string

// The environments a node can run in.
const (
	EnvironmentProduction  Environment = "production"
	EnvironmentStaging     Environment = "staging"
	EnvironmentDevelopment Environment = "development"
)

// Node identifies a node and controls its lifecycle.
type Node struct {
	// ID is the node's stable identity; in Kubernetes, derived from the pod
	// ordinal.
	ID string `koanf:"id"`
	// Environment is the deployment kind; development unlocks unsafe
	// settings (KV-SEC-041).
	Environment Environment `koanf:"environment"`
	// DataDir is the directory holding the command log and engine data.
	DataDir string `koanf:"data_dir"`
	// ShutdownGrace bounds graceful shutdown; it must be shorter than the
	// Kubernetes terminationGracePeriodSeconds (KV-CFG-011).
	ShutdownGrace time.Duration `koanf:"shutdown_grace"`
}

// Listen holds the addresses the node binds to, as host:port.
type Listen struct {
	// Client is the gRPC API listener.
	Client string `koanf:"client"`
	// Peer is the Raft transport listener.
	Peer string `koanf:"peer"`
	// Metrics is the Prometheus listener, never exposed publicly
	// (KV-OBS-010).
	Metrics string `koanf:"metrics"`
	// Diagnostics is the pprof listener; empty disables it (KV-CFG-014).
	Diagnostics string `koanf:"diagnostics"`
}

// Advertise holds the addresses announced to other nodes and clients, as
// host:port. An empty address means the matching listen address.
type Advertise struct {
	// Client is the address clients use to reach this node.
	Client string `koanf:"client"`
	// Peer is the address other nodes use to reach this node.
	Peer string `koanf:"peer"`
}
