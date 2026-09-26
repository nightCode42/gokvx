# 0005. Pebble as the storage engine

- **Status:** Accepted
- **Date:** 2026-09-26
- **Requirements:** `KV-STO-001`, `KV-STO-005`, `KV-STO-020`, `KV-DAT-010`

## Context and problem

The state machine needs an embedded, ordered key-value engine: MVCC reads seek by key and revision, scans walk key ranges in unsigned byte order (`KV-DAT-010`), and every applied command must land atomically together with the applied index, so recovery can replay the command log idempotently (`KV-STO-003`). The engine runs inside the gokvx process, is written by one apply loop, and must survive crashes without manual repair. The specification requires an LSM engine behind an `Engine` interface, with `cockroachdb/pebble` as the reference implementation (`KV-STO-001`).

## Decision drivers

- Correct under crashes: atomic batches, its own write-ahead log, no manual recovery.
- Ordered iteration with range bounds and consistent snapshots.
- Pure Go: no cgo, so builds stay static and cross-compile for `linux/amd64` and `linux/arm64` (`DEP-001`, `DEP-002`).
- Suited to the write pattern: an append-heavy stream of small versioned writes.
- Production-proven and maintained.

## Considered options

1. **Pebble** (`github.com/cockroachdb/pebble`)
2. **BadgerDB** (`github.com/dgraph-io/badger`)
3. **bbolt** (`go.etcd.io/bbolt`)

## Decision

Chosen option: **Pebble**, behind the `storage.Engine` interface, which keeps it replaceable in tests by the in-memory `storagetest.MemEngine`; both pass the same contract tests.

- Pebble is the storage engine of CockroachDB, so it is proven at scale under exactly this kind of load, and it is pure Go.
- Its LSM design turns the stream of small versioned writes into sequential I/O.
- It offers atomic batches, bounded iterators, and consistent snapshots, which is everything the MVCC store needs.
- Its format version is pinned explicitly (`FormatValueSeparation`) instead of Pebble's default of the oldest supported format, so any format upgrade is a deliberate, reviewed change.
- Its log messages are routed through the node's structured logger, so the process writes only JSON logs (`KV-OBS-020`).

## Consequences

- **Positive:** Crash consistency of the state comes from Pebble's atomic batches and its WAL; gokvx only has to get the command log right. Static, cgo-free builds.
- **Negative:** Pebble has a large dependency tree, and an LSM engine brings write amplification and background compactions whose I/O must be accounted for in benchmarks (`QA-032`).
- **Follow-up:** The MVCC key layout inside Pebble is decided in ADR-0004. Engine metrics (`gokvx_db_size_bytes`, compaction duration) come with the metrics work.

## Options in detail

### Pebble

Meets every driver. Pebble's API is designed for exactly this use — CockroachDB's MVCC layer — and it is actively maintained by Cockroach Labs.

### BadgerDB

Pure Go and fast, with key-value separation that suits large values. Rejected: its value log needs separate garbage collection that the application must drive, adding an operational task and a second place for space to leak, and its own MVCC layer overlaps with gokvx's, adding complexity without benefit.

### bbolt

The engine behind etcd: pure Go, simple, with a single B+tree file and serializable transactions. Rejected: writes rewrite B+tree pages in place and are serialized by a single writer lock, which makes it weaker for an append-heavy workload, and its file never shrinks without an offline compaction.
