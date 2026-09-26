package config

import "time"

// FsyncPolicy selects when the command log is flushed to disk (KV-STO-005).
type FsyncPolicy string

// The fsync policies.
const (
	// FsyncAlways flushes before acknowledging each write; the default.
	FsyncAlways FsyncPolicy = "always"
	// FsyncInterval flushes every fsync_interval; a crash can lose the
	// writes acknowledged since the last flush.
	FsyncInterval FsyncPolicy = "interval"
	// FsyncOS leaves flushing to the operating system; a crash can lose any
	// write the OS has not yet flushed.
	FsyncOS FsyncPolicy = "os"
)

// Storage configures the storage engine and the command log (spec §9).
type Storage struct {
	// Engine is the storage engine; "pebble" is the only one.
	Engine string `koanf:"engine"`
	// Fsync is the command log's flush policy.
	Fsync FsyncPolicy `koanf:"fsync"`
	// FsyncInterval is the flush period when Fsync is "interval".
	FsyncInterval time.Duration `koanf:"fsync_interval"`
	// WALSegmentBytes is the size of each command log segment (KV-STO-002).
	WALSegmentBytes int64 `koanf:"wal_segment_bytes"`
	// WALRetainSegments is how many segments are kept beyond the latest
	// snapshot (KV-STO-008).
	WALRetainSegments int `koanf:"wal_retain_segments"`
	// SnapshotEntries triggers a snapshot after this many applied entries
	// (KV-STO-006).
	SnapshotEntries int `koanf:"snapshot_entries"`
	// SnapshotInterval triggers a snapshot after this much time.
	SnapshotInterval time.Duration `koanf:"snapshot_interval"`
	// QuotaBytes is the database size above which writes are refused
	// (KV-STO-010).
	QuotaBytes int64 `koanf:"quota_bytes"`
	// AutoCompactionRetention is how many revisions automatic compaction
	// keeps; zero disables it (KV-DAT-007).
	AutoCompactionRetention int64 `koanf:"auto_compaction_retention"`
}
