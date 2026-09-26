// Package config loads, validates, and describes the configuration of a gokvx
// node (spec §13, Appendix D).
//
// # Sources and precedence
//
// A configuration is assembled from four layers, each overriding the ones
// before it (KV-CFG-001):
//
//  1. built-in defaults, from [Defaults];
//  2. an optional YAML file;
//  3. GOKVX_* environment variables;
//  4. command-line flags registered by [RegisterFlags].
//
// Every layer uses the same keys, such as storage.fsync, which appears as
// GOKVX_STORAGE_FSYNC in the environment and --storage.fsync on the command
// line. A key that does not exist is an error, never ignored, so a typo in a
// security setting cannot silently fall back to its default (KV-CFG-020).
//
// # Using this package
//
// [Load] assembles a [Config] and rejects unknown keys and values of the wrong
// type. [Config.Validate] then checks every rule and reports all violations at
// once, each naming its key (KV-CFG-002). [Config.Warnings] lists the settings
// that weaken safety (KV-CFG-021), and [Config.Redacted] returns a copy that
// is safe to log (KV-CFG-006).
//
// The reference for every key is docs/configuration.md; tests keep it, and
// the defaults in spec Appendix D, identical to this package.
package config
