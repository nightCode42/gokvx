# System Invariants

The technical rules gokvx must never violate, condensed from the specification with requirement references. When implementing an area, read its section here and the referenced spec sections — the spec remains authoritative.

Phase tags mark when a rule becomes binding. Rules without a tag apply from Phase 1.

---

## 1. Consistency model

- gokvx is **CP** (`ADR-0001`). A node that cannot confirm it holds a quorum rejects writes and linearizable reads rather than serving possibly stale or divergent data.
- `Put`, `Delete`, `CompareAndSwap`, `Compact`, and `Txn` are linearizable: once a response is returned, every later linearizable read observes the effect (`KV-API-015`).
- Read consistency modes (spec §6.4, `KV-API-011`):

| Mode | Behavior |
|---|---|
| `LINEARIZABLE` (default) | P1: served after all committed commands are applied. P2+: ReadIndex — confirm leadership with a quorum heartbeat, wait until `applied_index ≥ read_index`, then serve. |
| `SERIALIZABLE` | Served from the local replica's applied state with no coordination. Possibly stale. |
| `STALE` | As `SERIALIZABLE`, but if `min_revision` is set and the local applied revision is lower, fail with `FAILED_PRECONDITION`. |

- `CONSISTENCY_UNSPECIFIED` is treated as `LINEARIZABLE` — the safest interpretation, never the cheapest (`KV-API-080`).
- Every response carries a `ResponseHeader` with `cluster_id`, `member_id`, `revision`, and `raft_term` (`KV-API-004`); from P3 also `slot_map_version` and `shard_group`. Clients achieve read-your-writes by threading revisions.

## 2. The command path

- Every state mutation is a serialized command, appended to the `CommandLog`, applied by the apply loop to the `StateMachine` (`KV-STO-000`). P1 supplies a file-backed `CommandLog`; P2 replaces it with the Raft log behind the **same interface**. No mutation bypasses this path — not compaction, not lease expiry, not administrative operations.
- The apply loop is single-threaded per group and applies entries strictly in index order (`KV-CON-011`).
- **Determinism (A-2).** Given the same command sequence, every replica produces byte-identical state. Wall-clock time, random values, map iteration order, and hostnames never influence applied state. Time-dependent values (e.g. a lease expiry instant) are chosen by the proposer and carried inside the command.

## 3. MVCC

Source: spec §7.1, `ADR-0004`.

- The **revision** is a 64-bit counter per shard group, incremented **exactly once per committed command that writes at least one key** — a value or a tombstone (`KV-DAT-001`):

| Consumes one revision | Consumes no revision |
|---|---|
| `Put`, including one that rewrites the current value, and `Put` with `ignore_value` | Every read: `Get`, `List`, `Watch`, `Status` |
| `Delete` that removes at least one key | `Delete` that matches no key |
| `CompareAndSwap` whose comparison holds | `CompareAndSwap` whose comparison fails |
| `Txn` whose executed branch writes — one revision for the whole transaction | `Txn` whose executed branch writes nothing |
| `LeaseRevoke`, or lease expiry, that deletes attached keys | `LeaseRevoke` with no attached keys, `LeaseGrant`, `LeaseKeepAlive` |
| | `Compact` |

- The current revision is persisted explicitly in engine metadata and snapshots, never derived from the key space — compaction and deletion make `max(mod_revision)` unreliable (`KV-DAT-008`).
- A response to a command that consumed no revision carries the current revision in its header.
- Each key entry records `create_revision`, `mod_revision`, `version`, `value`, and `lease_id` (`KV-DAT-002`).
  - `version` starts at 1 when a key is created, increments on each write, and resets to 0 on delete.
  - A key recreated after deletion starts a new generation with a new `create_revision`.
- A delete writes a **tombstone** at a new revision; history is not erased (`KV-DAT-003`), so watches observe deletions.
- Reads accept `revision` and serve a consistent historical snapshot at it (`KV-DAT-004`). A revision below the compaction point fails with `OUT_OF_RANGE` (`KV-DAT-005`).
- `Compact` requires `kv:admin` and is itself a replicated command, so all replicas compact identically (`KV-DAT-006`).
- Property invariants verified by tests (`QA-004`): `mod_revision` never decreases per key; a read at revision *R* is unaffected by writes above *R*; `version` is consistent with the key's write history.

## 4. Keys and values

Source: spec §7.2.

- Keys are opaque byte strings ordered lexicographically by **unsigned** byte value (`KV-DAT-010`). No encoding, normalization, or case folding.
- Defaults: key ≤ 1 KiB, value ≤ 1 MiB, request ≤ 4 MiB — all configurable. Exceeding a limit returns `INVALID_ARGUMENT` (`KV-DAT-011`, `KV-DAT-013`).
- An empty key is rejected with `INVALID_ARGUMENT`, except as a range boundary (`KV-DAT-012`).
- A `range_end` of a single `0x00` byte means "to the end of the key space" (`KV-API-020`). Ranges are half-open: `[start, end)`.

## 5. Slots

Source: spec §7.3, `ADR-0003`. **The slot function is permanent.** Changing it would silently misroute every existing key.

- The key space has a fixed slot count, set at bootstrap and immutable afterwards; default `16384` (`KV-DAT-020`).
- `slot = crc32c(hash_input) mod slot_count`, using the Castagnoli polynomial (`KV-DAT-021`).
- `hash_input` is the substring between the **first** `{` and the **first following** `}` if that substring is non-empty; otherwise the whole key.

| Key | Hash input |
|---|---|
| `/app/config` | `/app/config` |
| `{tenant-42}/users/1` | `tenant-42` |
| `{}/x` | `{}/x` (empty braces are not a partition key) |

- The function is implemented and golden-tested in P1 even though P1 has one group (`KV-DAT-022`, `QA-006`).
- A prefix scan is single-group — cheap and ordered — only if the prefix contains a complete partition key.

## 6. Operations

### Get (`KV-API-010`–`011`)

- An absent key returns `count = 0`, **never** a gRPC error.
- Supports `consistency`, `revision`, `keys_only`, and `min_revision` (STALE only).

### Put and Delete (`KV-API-012`–`014`)

- `Put` returns the new revision and, if `prev_kv` is set, the previous pair.
- `Put` with `ignore_value` updates only the attached lease; if the key does not exist it fails with `INVALID_ARGUMENT`.
- `Delete` removes one key or a range, returns the count deleted, and optionally the deleted pairs.

### CompareAndSwap (`KV-API-030`–`033`)

- The comparison and the write are **one replicated command** — never a read followed by a write.
- Targets: `VALUE`, `VERSION`, `CREATE_REVISION`, `MOD_REVISION`, `LEASE`. Operators: `EQUAL`, `NOT_EQUAL`, `GREATER`, `LESS`. An `UNSPECIFIED` target or operator fails with `INVALID_ARGUMENT`.
- `MOD_REVISION EQUAL 0` means "the key does not exist" — create-if-absent.
- A failed comparison is a **successful RPC** with `succeeded = false` and the current key state. It is never an error, and it consumes no revision (`KV-DAT-001`).

### List and pagination (`KV-API-020`–`025`)

- A request selects either a `prefix` or a `range` — exactly one, enforced by `oneof`.
- Supports `limit`, `keys_only`, `count_only`, sort order and target, and `min_mod_revision` / `max_mod_revision`.
- The page token is opaque and integrity-protected (HMAC). It carries the snapshot revision and per-group cursors; clients can never depend on its format.
- Every page is served at the snapshot revision fixed by the first page, so a paginated scan is a consistent point-in-time view. If that revision is compacted mid-scan, fail with `OUT_OF_RANGE`.
- P3: a range spanning groups is a scatter-gather that merges results in the requested order and reports `groups_queried`.

## 7. Watch

Source: spec §8.5.

- One bidirectional stream carries many logical watches, each created and canceled by client-assigned `watch_id` (`KV-API-040`).
- A watch selects a key, a prefix, or a range — exactly one (`KV-API-041`).
- With `start_revision`, historical events are replayed first, in revision order, **with no gaps**, before live events (`KV-API-042`).
- If `start_revision` is below the compaction point, the watch receives a response with `canceled = true` and `compact_revision` set; events are never skipped silently. This is a watch response, not a gRPC error — the stream stays open (`KV-API-043`).
- Events arrive in non-decreasing revision order, and **all events of one revision arrive in one message**, so a transaction is observed atomically (`KV-API-044`).
- Per-stream buffering is bounded. A consumer too slow for the limit has its watch canceled with `RESOURCE_EXHAUSTED`; memory never grows unbounded (`KV-API-045`).
- A stream is terminated when its authenticating token expires (`KV-SEC-026`).
- P3: multi-group watches guarantee ordering per group only, and each event reports its originating group (`KV-API-047`).

## 8. Storage and durability

Source: spec §9. There are **two logs** with different jobs: the command log records *intent* (what must be applied); the Pebble engine's own WAL records *state*. The exact byte layout and recovery rules are in [storage-format.md](storage-format.md).

- **Engine.** Pebble, behind an `Engine` interface so tests can substitute it (`KV-STO-001`).
- **Command log format.** Segmented files of configurable size. Each record is length-prefixed and protected by a CRC32C checksum over its payload (`KV-STO-002`).
- **Recovery.** Open the engine, read the persisted `applied_index`, replay every record above it. Replay is idempotent: replaying the same records twice yields identical state (`KV-STO-003`).
- **Corruption.** A torn or corrupt record at the **tail** is truncated and logged at `WARN` with its offset. A checksum failure **mid-log**, followed by valid records, aborts startup with a non-zero exit (`KV-STO-004`).
- **fsync.** `always` (default: fsync before acknowledging), `interval`, or `os`. Anything but `always` logs a startup `WARN` (`KV-STO-005`).
- **Snapshots.** Taken after a configurable number of applied entries (default 10,000) or interval (`KV-STO-006`). Written atomically: temp file → fsync → rename → fsync the directory. A partial snapshot is never selectable (`KV-STO-007`).
- **Retention.** Segments fully covered by a durable snapshot become deletable; a configurable number are kept beyond that (`KV-STO-008`).
- **Format versioning.** A `storage_version` marker file; startup fails fast on an incompatible version (`KV-STO-009`).
- **Quota.** Above the configured database size, writes fail with `RESOURCE_EXHAUSTED` while reads continue, and an alertable metric is raised (`KV-STO-010`).
- **Verification.** SIGKILL-at-random-points crash tests, torn-write tests, and bit-flip tests (`KV-STO-020`–`022`): every acknowledged write survives; no unacknowledged write is partially applied.

## 9. Consensus (P2)

Source: spec §10.1–10.2, `ADR-0002`.

- `etcd-io/raft` provides the algorithm. gokvx owns storage, transport, the tick loop, and the state machine — and never reimplements the algorithm (`KV-CON-001`).
- **Persist before send:** `HardState` and new entries are durable before the corresponding messages go to peers (`KV-CON-012`).
- PreVote and CheckQuorum are always enabled (`KV-CON-005`, `KV-CON-006`).
- Linearizable reads use ReadIndex; a leader never serves them from local state without confirming leadership (`KV-CON-007`).
- A follower forwards writes to the leader by default, or rejects with `FAILED_PRECONDITION` and the leader's address when configured to (`KV-CON-008`).
- With no known leader, requests fail with `UNAVAILABLE` after a bounded wait, respecting the caller's deadline (`KV-CON-009`).
- Every proposal carries a unique request ID; a bounded deduplication window ensures a retried proposal is applied at most once (`KV-CON-010`).
- Election timeout ≥ 10 × heartbeat interval (`KV-CON-004`). Groups have an odd size; a group of *N* tolerates `floor((N−1)/2)` failures (`KV-CON-014`).
- No committed write is ever lost or reordered across leader changes — verified by the linearizability harness (`KV-CON-023`, `QA-020`–`024`).

## 10. Sharding, membership, leases, transactions (P3–P4)

- **Slot map (P3).** An authoritative, replicated slot-to-group map with a monotonically increasing version. Every node resolves any key and forwards or redirects per configuration (`KV-CON-030`–`034`). A P1/P2 data directory is readable by a P3 binary without migration.
- **Membership (P4).** Joint consensus (`ConfChangeV2`) only. New members join as learners and are promoted once caught up. Changes that would lose quorum are refused (`KV-CON-040`–`044`).
- **Slot migration (P4).** `STABLE → MIGRATING → HANDOVER → STABLE`, each transition in the replicated slot map. Writes are never lost, duplicated, or reordered; a crashed migration recovers with the slot owned by exactly one group (`KV-CON-050`–`055`).
- **Leases (P4).** Expiry is enacted only by the leader proposing a replicated `LeaseRevoke`; followers never expire leases from their own clock. Revocation deletes all attached keys in one command. A new leader resets all lease timers to a full TTL (`KV-API-060`–`065`).
- **Transactions (P4).** One shard group only — otherwise `FAILED_PRECONDITION` with `CROSS_SHARD_TXN_UNSUPPORTED`. One replicated command, one shared revision, nesting depth ≤ 8 (`KV-API-050`–`053`). Authorization considers the transaction's *potential* effect: any mutating operation requires `kv:write`, even if the executed branch only reads (`KV-SEC-024`).

## 11. Security

Source: spec §11, `ADR-0006`. **Two independent controls on every request:** mTLS proves *which workload* is calling; the JWT proves *which principal* acts and *with what authority*. Neither substitutes for the other.

### Transport

- mutual TLS on every client and peer listener; TLS 1.3 minimum; `RequireAndVerifyClientCert` (`KV-SEC-001`–`003`).
- Peer and client authorization match the certificate **SAN** against an allowlist — never the Common Name (`KV-SEC-004`).
- Certificates and CA bundles hot-reload without a restart, logging the new fingerprint and expiry; certificate expiry is exported as a metric (`KV-SEC-005`–`006`).

### Tokens

- Bearer JWT in the `authorization` metadata key. No API keys, shared secrets, or basic auth (`KV-SEC-010`).
- Verified locally against cached JWKS keys; never a synchronous call to the issuer on the request path (`KV-SEC-011`).
- JWKS refreshes in the background (default 15 min). A failed refresh keeps the last good key set and raises a metric and `WARN` (`KV-SEC-012`).
- An unknown `kid` triggers at most one out-of-band refresh, rate-limited (default 60 s) so a token flood cannot be amplified against the issuer (`KV-SEC-013`).
- Algorithms come from configuration (default `["RS256"]`), never from the token header; `alg: none` and anything outside the set are rejected immediately (`KV-SEC-014`).
- `iss`, `aud`, `exp`, `nbf`, `iat` validated with configurable skew (default 60 s). Tokens without `exp` or `kid` are rejected (`KV-SEC-015`–`016`).
- Verified claims may be cached under a keyed hash of the token, bounded with LRU eviction, and expiring no later than the token (`KV-SEC-017`, `KV-SEC-036`).
- JWKS is fetched over verified HTTPS with a timeout, size limit, and minimum RSA modulus of 2048 bits (`KV-SEC-018`). With auth enabled, startup fails if the JWKS endpoint is unreachable within a bounded window (`KV-SEC-019`).

### Authorization

- A unary and a stream interceptor apply a declarative method-to-scope table to every method; a method missing from the table is denied (`KV-SEC-020`).
- Scopes: `kv:read`, `kv:write`, `kv:watch`, `kv:admin`, read from the space-delimited `scope` claim. `kv:write` does not imply `kv:read` (`KV-SEC-021`–`022`).

| Scope | Methods |
|---|---|
| `kv:read` | `Get`, `List` |
| `kv:write` | `Put`, `Delete`, `CompareAndSwap`, `Txn`, lease operations except `LeaseTimeToLive` |
| `kv:watch` | `Watch` |
| `kv:admin` | `Compact`, `MemberAdd`, `MemberRemove`, `MemberUpdate`, `MemberPromote`, `MoveSlots` |
| authenticated only | `Status`, `MemberList`, `ShardMap`, `LeaseTimeToLive`, health |

- Streams are authorized at establishment and terminated when the token expires (`KV-SEC-026`).

### Hardening

- Authentication and authorization failures disclose nothing: not whether a key exists, not which claim failed (`KV-SEC-030`).
- Tokens, signatures, key material, keys, and values never appear in logs, traces, metric labels, or error messages (`KV-SEC-031`).
- Per-principal token-bucket rate limits by operation class; stream and watch counts bounded (`KV-SEC-032`–`033`).
- Containers run as non-root with a read-only root filesystem and all capabilities dropped (`KV-SEC-034`).
- A security audit log records authentication failures, denials, admin operations, and certificate reloads (`KV-SEC-035`).

### Development mode

`auth.mode: disabled` requires `environment: development`, logs a banner on every start, sets `gokvx_auth_disabled = 1`, and binds only to loopback unless `insecure_allow_remote: true` is also set — three explicit settings before an unauthenticated node is reachable (`KV-SEC-040`–`043`).

## 12. Errors

The model is defined in [error-handling.md](error-handling.md). The invariants:

- Every error returned to a client carries a gRPC code plus `google.rpc.ErrorInfo` with `domain = "gokvx.io"` and a `reason` from the fixed, documented enumeration (`KV-API-090`, `KV-API-092`).
- Retryable errors carry `google.rpc.RetryInfo` (`KV-API-091`).
- `INTERNAL` is always accompanied by an `ERROR` log.
- Absent keys and failed comparisons are not errors.

## 13. Observability

Source: spec §12, Appendix C.

- **Traces.** OpenTelemetry via OTLP; W3C Trace Context propagated in and out, including inter-node traffic. Each operation has child spans for authentication, authorization, slot resolution, proposal or forwarding, replication wait, and apply (`KV-OBS-001`–`003`).
- **Span attributes** include method, a **hash** of the key prefix (never the raw key), principal, consistency mode, group, term, revision, and status (`KV-OBS-004`).
- **Metrics.** Names, types, and labels exactly as in Appendix C, on a private listener. Latency is always a histogram with explicit buckets, never a summary. Label values are bounded — never keys, prefixes, unbounded subjects, or raw error strings (`KV-OBS-010`–`013`).
- **Logs.** JSON via `log/slog`, one event per line, with `trace_id`, `span_id`, `request_id`, `principal`, `method`, `node_id`. Per-request success is `DEBUG`; `INFO` is for lifecycle and state changes (`KV-OBS-020`–`024`).
- **Health.** Liveness (`""`) reports only process responsiveness — never quorum, leadership, or peers. Readiness (`gokvx.readiness`) requires an open engine, a bounded apply lag, and a known leader. Health checks never issue a consensus round or a disk write (`KV-OBS-030`–`034`).

## 14. Configuration and lifecycle

Source: spec §13, Appendix D.

- Precedence: flags > environment (`GOKVX_*`) > file > defaults (`KV-CFG-001`).
- Validation runs at startup; an invalid setting exits non-zero naming the offending key. **Unknown keys are rejected**, so a typo in a security setting cannot silently fall back to a default (`KV-CFG-002`, `KV-CFG-020`).
- `gokvx config validate` checks a file without starting the server (`KV-CFG-003`).
- Secrets are never accepted as flags; the effective configuration is logged with secrets redacted (`KV-CFG-005`–`006`).
- Each safety-weakening setting logs its own distinct `WARN` naming the setting and its consequence (`KV-CFG-021`).
- On `SIGTERM`: readiness goes `NOT_SERVING`, new RPCs stop, in-flight unary calls get a grace period, streams close with `UNAVAILABLE`, exporters flush, the engine closes — all within a deadline shorter than Kubernetes' `terminationGracePeriodSeconds` (`KV-CFG-010`–`011`).

## 15. Wire contract

Source: spec §4.3, §8.1, Appendix A.

- The contract is complete in Phase 1, including every field later phases need (`KV-API-000`).
- After the Phase 1 tag, no field is added, removed, renumbered, or changed in meaning. `buf breaking` against the previous release fails CI on any violation (`KV-API-003`).
- Enum zero values are `*_UNSPECIFIED` or a documented safe default; mutually exclusive fields use `oneof`; removed field numbers are `reserved` and never reused (`KV-API-080`–`082`).
- Generated code is committed (`KV-API-002`).

## 16. microservice-1

Source: spec §14–15, Appendix B.2.

- **Token lifecycle.**
  - Obtain a service token at startup and hold it in memory only.
  - Refresh proactively at 50% of its lifetime with ±10% jitter.
  - Refresh reactively on `UNAUTHENTICATED` and retry the original request exactly once — never on `PERMISSION_DENIED`.
  - Collapse concurrent refreshes with single-flight; back off with full jitter behind a circuit breaker (`MS1-SEC-001`–`009`).
- A refresh failure never affects serving while a valid token is cached; readiness fails only when no valid token exists (`MS1-SEC-007`).
- The token is attached per RPC via `PerRPCCredentials`, read at call time (`MS1-API-025`).
- gRPC client: round-robin over the headless service's DNS, compatible keepalives, deadlines derived from the HTTP request (`MS1-API-021`–`023`).
- Retries only on `UNAVAILABLE` / `DEADLINE_EXCEEDED`, only for idempotent operations; `Put` is retried only when made idempotent by `If-Match` (`MS1-API-024`).
- HTTP mapping follows Appendix B.2 exactly. `PERMISSION_DENIED` from gokvx means *our* service account is misconfigured and becomes `502`, never `403`; `403` is reserved for a caller writing outside the demo namespace (`DEM-011`).
