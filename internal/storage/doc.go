// Package storage holds gokvx's two durable structures (spec §9): the
// command log, which records the intent of every mutation, and the Engine,
// in which the state machine keeps its data. They serve different purposes
// and are never merged.
//
// # Command log
//
// [Log] is the file-backed command log of Phase 1 (KV-STO-000, KV-STO-002).
// Records are appended in index order to segment files, each record
// length-prefixed and protected by a CRC32C checksum. A write is acknowledged
// only once it is durable under the configured fsync policy (KV-STO-005). On
// open, the log verifies every record: a damaged tail — a record cut short by
// a crash — is truncated with a warning, while damage followed by intact
// records is corruption and refuses to open (KV-STO-004). The exact format is
// in docs/engineering/storage-format.md.
//
// # Engine
//
// [Engine] is an ordered key-value store with atomic batches (KV-STO-001).
// [Pebble] implements it on github.com/cockroachdb/pebble; package
// storagetest provides an in-memory implementation for tests, and both pass
// the same contract tests.
//
// # Data directory
//
// [PrepareDataDir] checks the storage_version marker of a data directory
// before anything else in it is read (KV-STO-009).
//
// On-disk formats are permanent once released. Any change to them requires
// the maintainer's approval (internal/storage/AGENTS.md).
package storage
