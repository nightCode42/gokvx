# internal/consensus — Agent Notes

The Raft node (Phase 2+): storage for the Raft log, the peer transport, the tick loop, and the apply loop, built on `go.etcd.io/raft/v3`. This package decides whether a committed write can ever be lost. Read spec §10.1–10.2 and [system-invariants §9](../../docs/engineering/system-invariants.md#9-consensus-p2) before editing.

## Invariants

- The Raft algorithm comes from `etcd-io/raft` and is never reimplemented or patched (`KV-CON-001`).
- Process each `Ready` in the order the library documents: persist `HardState`, entries, and snapshot **before** sending messages to peers (`KV-CON-012`), apply committed entries, then call `Advance`. Verify every API against the library's source and documentation — never from memory.
- PreVote and CheckQuorum are always enabled (`KV-CON-005`, `KV-CON-006`).
- Linearizable reads use ReadIndex and wait until `applied_index ≥ read_index` (`KV-CON-007`).
- The apply loop is single-threaded per group and applies in index order (`KV-CON-011`).
- Proposals carry unique request IDs; the deduplication window is bounded and documented (`KV-CON-010`).
- This package never imports the transport layer (`internal/server`) (A-3).

## Stop and ask before

- Changing the order of persistence, sending, and applying.
- Changing timing defaults (tick, heartbeat, election) or disabling PreVote or CheckQuorum.

## Required tests

- Failover, catch-up by log and by snapshot, and no-leader behaviour (`KV-CON-020`–`023`).
- The linearizability harness under every fault scenario in `QA-022`, with zero violations.
- Election timing driven by a fake clock, never by real time.
