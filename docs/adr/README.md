# Architecture Decision Records

Significant design decisions are recorded here using the [MADR](https://adr.github.io/madr/) format ([template](template.md)). An ADR is immutable once accepted; reversing a decision means writing a new ADR that supersedes it.

Write an ADR when a decision is hard to reverse, affects more than one package, adds or replaces a dependency ([dependencies.md](../engineering/dependencies.md)), or chooses between reasonable alternatives that a future reader would question.

Files are named `NNNN-short-title.md` with a four-digit, never-reused number.

## Index

| ADR | Title | Status |
|---|---|---|
| 0001 | CAP positioning: gokvx is a CP system | Planned |
| 0002 | Raft library rather than a bespoke implementation | Planned |
| 0003 | Fixed slot space rather than a consistent hash ring | Planned |
| 0004 | MVCC revision model | Planned |
| 0005 | Pebble as the storage engine | Planned |
| 0006 | mTLS and JWT as complementary controls | Planned |
| 0007 | GKE Autopilot versus Standard | Planned |
| 0008 | gRPC as the sole client protocol | Planned |
| 0009 | Single-shard transactions only | Planned |
| 0010 | Phased delivery and the frozen wire contract | Planned |
| [0011](0011-unified-error-model.md) | One error type with a registered reason, translated only at the edges | Accepted |

The decisions for ADRs 0001–0010 are summarized in spec §21. Each ADR is written in full before or alongside the first implementation that depends on it, and its status is updated here.
