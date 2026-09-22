# internal/storage — Agent Notes

The `Engine` interface with its Pebble implementation, and the file-backed `CommandLog`. This package decides whether an acknowledged write survives a crash. Read spec §9 and [system-invariants §8](../../docs/engineering/system-invariants.md#8-storage-and-durability) before editing.

## Invariants

- Two logs, two jobs: the `CommandLog` records intent; Pebble's WAL records state. Never merge or confuse them.
- Every log record is length-prefixed and carries a CRC32C (Castagnoli) checksum over its payload (`KV-STO-002`).
- A write is acknowledged only after it is durable under the configured fsync policy; `always` is the default (`KV-STO-005`).
- Recovery replays every record above the persisted `applied_index` and is idempotent (`KV-STO-003`).
- A corrupt or torn **tail** is truncated with a `WARN` naming the offset; a corrupt record **mid-log** aborts startup (`KV-STO-004`). These two cases must never be confused.
- Snapshots are written to a temporary path, fsynced, renamed, and the directory fsynced. A partial snapshot is never selectable (`KV-STO-007`).
- The `storage_version` marker is checked before anything else is read (`KV-STO-009`).

## Stop and ask before

- Changing the record format, segment naming, snapshot layout, or `storage_version` semantics. On-disk formats are permanent once released.
- Changing fsync behaviour or the order of any write, fsync, or rename.

## Required tests

- Crash recovery under `SIGKILL` at random points, torn-write truncation, and bit-flip detection (`KV-STO-020`–`022`).
- Recovery idempotency: replaying the same records twice yields identical state.
- The in-memory `Engine` fake in `storagetest` passes the same contract tests as the Pebble implementation.
