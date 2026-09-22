# Product Context

What gokvx is, who it is for, and the boundaries that every design decision must respect. Source: spec §1–§5.

---

## 1. Mission

gokvx is a **distributed, strongly consistent key-value store** with a gRPC API, Raft replication, slot-based sharding, MVCC revisions, leases, and streaming watches.

It is built as a **reference implementation**: a system small enough to read end to end, yet complete enough to show how a production-grade distributed store handles consistency, durability, security, observability, and deployment. Its value is measured by two questions:

1. **Are its claims true?** Linearizability is checked with a model checker under fault injection; durability is checked by killing the process mid-write. A claim without that evidence is not made.
2. **Can an engineer who did not write it understand it?** Every non-obvious decision is explained in code comments, the handbook, or an ADR.

## 2. Positioning

gokvx does not claim to replace or outperform established systems, and nothing in code, documentation, or benchmarks may suggest otherwise.

| System | Relationship |
|---|---|
| etcd | The API model: revisions, watches, leases, compare-and-swap. gokvx follows it deliberately. |
| TiKV | The scaling model: many Raft groups over a partitioned key space (gokvx Phase 3). |
| Redis Cluster | The one idea borrowed: a fixed 16,384-slot key space. Redis is otherwise a different class of system (in-memory, asynchronous replication). |

Benchmarks exist for calibration against etcd under identical conditions (`QA-033`), never for marketing.

## 3. Design goals

When two good options conflict, the goal listed first wins.

| # | Goal | Consequence |
|---|---|---|
| G1 | Correctness before features | Linearizability is a verified property; the model-checking harness gates releases. |
| G2 | Provider-agnostic security | gokvx trusts any RFC 7517 JWKS issuer it is configured with and depends on none. |
| G3 | No synchronous auth on the hot path | Tokens are verified locally against cached keys. |
| G4 | Operable by someone who did not write it | Health, metrics, traces, logs, runbooks, and dashboards are requirements. |
| G5 | Identical topology locally and in production | Every Compose capability has a Kubernetes equivalent; one end-to-end suite runs against both. |
| G6 | Later phases never break earlier ones | The wire contract is complete at Phase 1 and frozen at its tag. |

The CAP position is fixed by `ADR-0001`: **gokvx is CP.** Under a partition, the minority side rejects writes and linearizable reads. Stale reads are an explicit, opt-in, detectable escape hatch — never a silent fallback.

## 4. Non-goals

A request that moves the system toward a non-goal is declined, with a reference to this table.

| ID | Non-goal | Instead |
|---|---|---|
| NG-1 | Cross-shard atomic transactions | Transactions are single-shard; cross-shard ones fail with `CROSS_SHARD_TXN_UNSUPPORTED`. |
| NG-2 | Secondary indexes or a query language | Range scans are the only query primitive. |
| NG-3 | Multi-region clusters | Single region, multiple zones. |
| NG-4 | Encryption at rest | Delegated to the storage layer (e.g. CMEK disks). |
| NG-5 | End-user authentication on the microservice-1 REST API | Terminated by an upstream gateway; the demo compensates with the controls in spec §17. |
| NG-6 | Changes to GoAuthx domains | Only the service-account contract in spec §15 is required. |
| NG-7 | Implementing Raft from scratch | `etcd-io/raft` is used (`ADR-0002`). |
| NG-8 | Compatibility with any pre-1.0 wire format | Nothing precedes the Phase 1 contract. |

## 5. Components and boundaries

```text
client ──HTTPS/REST──▶ microservice-1 ──gRPC + mTLS + JWT──▶ gokvx
                              │                                 ┆ (background, cached)
                              └──gRPC + mTLS (token only)──▶ identity provider ◀┘ JWKS
```

| Constraint | Rule |
|---|---|
| C-1 | gokvx contains no reference to GoAuthx. It knows only the configured `issuer_url` and `jwks_url`. |
| C-2 | The identity provider never calls gokvx or microservice-1. |
| C-3 | gokvx never calls an identity provider on the request path. |
| C-4 | microservice-1 never calls the identity provider while serving a REST request. |

**microservice-1 is deliberately thin.** It has no business logic, no datastore, and no value cache; every REST request becomes exactly one gokvx RPC (`MS1-CFG-001`). Its purpose is to show the correct way to consume gokvx and an identity provider together: token lifecycle, per-RPC credentials, deadlines, retries, and error mapping. Adding features to it defeats that purpose.

## 6. Delivery phases

| Phase | Theme | Delivers |
|---|---|---|
| P1 | Durable core | Single node, the complete gRPC contract, MVCC storage, crash-safe command log, mTLS + JWT, observability, microservice-1, Compose, Helm, GKE, CI |
| P2 | Replication | One Raft group, failover, ReadIndex reads, stale reads, TUI dashboard, linearizability verification |
| P3 | Horizontal scale | Slot-based sharding, routing, scatter-gather `List` and `Watch` |
| P4 | Elasticity | Joint-consensus membership, live slot migration, leases, single-shard transactions |

Rules that follow from phasing:

- Each phase ends in an independently deployable, fully tested state (spec §4.2). Nothing is left half-built across a phase boundary.
- Requirements become binding in the phase they are tagged with. Work on a later phase starts only when the maintainer says so.
- The wire contract is defined in full in Phase 1 (`KV-API-000`). Fields for later phases are accepted and answered with `UNIMPLEMENTED`, reason `FEATURE_NOT_IN_CURRENT_PHASE`.

## 7. Public demonstration environment

The system runs publicly so that anyone can exercise the REST API. Only microservice-1 and the issuer's JWKS endpoint are internet-facing; gokvx, the identity provider's gRPC interface, metrics, `pprof`, and Grafana never are (`DEM-001`). Writes are confined to a demonstration key namespace, limits are tightened, data is purged on a schedule, and the landing page states plainly that submitted data is public (spec §17). The legal obligations for an EU-hosted service (imprint and privacy notice, `DEM-030`–`031`) apply from the first public deployment.

## 8. Vocabulary

Use the spec's terms exactly; the [glossary](../requirements.md#23-glossary) is authoritative. In particular: a **revision** is the store's logical clock, a **version** is a per-key write counter, a **slot** is a partition of the key space, and a **shard group** is the Raft group that owns a set of slots. These words are never used interchangeably.
