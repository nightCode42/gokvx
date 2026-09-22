# internal/mvcc — Agent Notes

The multi-version store: revision index, historical reads, tombstones, and compaction. Every read in gokvx goes through this package. Read spec §7.1 and [system-invariants §3](../../docs/engineering/system-invariants.md#3-mvcc) before editing.

## Invariants

- The revision increments **exactly once per committed mutating command** — including no-op writes and whole transactions (`KV-DAT-001`).
- `create_revision`, `mod_revision`, `version`, `value`, and `lease_id` are recorded for every entry (`KV-DAT-002`). `version` starts at 1, increments per write, and resets to 0 on delete.
- Deletes write tombstones; history is never erased except by compaction (`KV-DAT-003`).
- A read at revision *R* sees exactly the state after *R* and is unaffected by any later write (`KV-DAT-004`).
- Reads below the compaction point fail with `REVISION_COMPACTED` (`OUT_OF_RANGE`) — never a partial or empty result (`KV-DAT-005`).
- Keys compare as unsigned bytes (`KV-DAT-010`).
- Applying a command is deterministic: no clock, no randomness, no map-order dependence (A-2).

## Stop and ask before

- Changing how revisions are assigned, or the on-disk encoding of the revision index.

## Required tests

- Property tests with `rapid` for the `QA-004` invariants: non-decreasing `mod_revision` per key, snapshot isolation of historical reads, and `version` consistent with write history.
- Compaction boundary cases: reads exactly at, above, and below the compaction revision.
