# gokvx

[![CI](https://github.com/nightCode42/gokvx/actions/workflows/ci.yml/badge.svg)](https://github.com/nightCode42/gokvx/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

A distributed, strongly consistent key-value store in Go: gRPC API, Raft replication, slot-based sharding, MVCC revisions, and streaming watches.

> **Status:** under active development, Phase 1 of 4. Not production-ready; the wire contract is frozen at the Phase 1 release.

## Design goals

- **Correctness first.** Linearizability is checked with a model checker (Porcupine) under fault injection — leader kills, partitions, process pauses — rather than asserted.
- **Security by default.** Mutual TLS between all workloads, plus JWT bearer tokens with scoped authorization, verified locally against the configured issuer's JWKS. No identity-provider call on the request path.
- **Operable.** Metrics, traces, structured logs, health checks, and runbooks are requirements, not afterthoughts.
- **Deployment parity.** The same topology and end-to-end suite run on Docker Compose and on Kubernetes (GKE).

## Components

| Component | Role |
|---|---|
| **gokvx** | The key-value store. gRPC only; MVCC storage on Pebble; consensus via `etcd-io/raft`. |
| **microservice-1** | A thin REST service showing how a consumer integrates with gokvx and an identity provider securely. |
| **Identity provider** | gokvx trusts only the JWT issuer and JWKS endpoint set in its configuration. The reference deployment uses [GoAuthx](https://github.com/nightCode42/goauthx); tests use a stub issuer. |

## Roadmap

- [ ] **Phase 1 — Durable core:** single node, full gRPC contract, crash-safe storage, mTLS + JWT, observability, Docker Compose, CI
- [ ] **Phase 2 — Replication:** Raft group, leader failover, linearizable reads, linearizability verification
- [ ] **Phase 3 — Horizontal scale:** slot-based sharding, multi-group routing, scatter-gather List/Watch
- [ ] **Phase 4 — Elasticity:** dynamic membership, slot rebalancing, leases, transactions

## Development

Prerequisites: Go (version in `go.mod`), GNU Make, Git, and Python for [pre-commit](https://pre-commit.com).

```bash
make setup   # install pinned tools and git hooks (once after cloning)
make check   # run every quality gate CI runs
make build   # build all binaries into bin/
make help    # list all targets
```

Every commit is checked by git hooks (formatting, linting, secret scanning, Conventional Commits), and every pull request runs the same gates in CI plus race-enabled tests and a vulnerability scan. AI-assisted tools are part of the development workflow; their output goes through the same review and quality gates as any other change.

## Documentation

- [Requirements specification](docs/requirements.md) — the full design, phased requirements, and verification strategy
- [Engineering handbook](docs/engineering/README.md) — code standards, error model, testing, dependencies, and workflow
- [Architecture decisions](docs/adr/README.md)
- [Working agreement](AGENTS.md) — the rules every contributor and AI agent follows
- [Contributing](CONTRIBUTING.md) · [Security policy](SECURITY.md)

## License

[Apache License 2.0](LICENSE)
