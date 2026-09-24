# Work Log

Current focus, hand-off notes, and open decisions. This file lets any contributor — or an AI agent starting a fresh session — pick up the work exactly where it stopped.

Per-requirement implementation status is **not** tracked here; it lives only in the `Status` column of [requirements.md](requirements.md) (spec §3.3).

---

## Current focus

| Item | Value |
|---|---|
| Phase | Phase 1 — Durable core |
| Active branch | `feat/shard-slot` |
| Active work | Slot function in `internal/shard` — complete, awaiting the maintainer's commit and pull request |
| Requirement IDs | `KV-DAT-020`–`022`, `QA-006` |

## Next up

In order. Each item is one branch and one pull request.

1. **Logging** — `internal/observability` logging: `slog` JSON handler, the standard request fields, level configuration, redaction rules; a `slog.LogValuer` on `kverr.Error`. Spec §12.3.
2. **Configuration** — `internal/config`: Appendix D structs, koanf loading with unknown keys rejected, per-section validation, `gokvx config validate`. `KV-CFG-001`–`006`, `KV-CFG-020`–`021`.
3. **Spelling** — settle on US English across the documentation, matching the linter's rule for Go code.

## Open decisions

| Decision | Owner | Notes |
|---|---|---|
| GoAuthx link in README | Maintainer | Link currently returns 404; publish GoAuthx or remove the link before sharing the repository. |

## Hand-off notes

- 2026-09-24 — Slot function built in `internal/shard`: `NewSlotter` validates the slot count once (zero fails with `INVALID_ARGUMENT`), `Slotter.Slot` is check-free and allocation-free, `HashInput` applies the brace rule without copying. Maintainer decisions: a positive count is a constructor precondition rather than a per-call check; a `Slotter` type instead of a bare function. The hash is anchored to published CRC32C values (check value and RFC 3720 vectors), 45 golden keys are locked in `testdata/slots.golden`, and a 45-second fuzz run (1.65M inputs) found no failures. `KV-DAT-021`, `KV-DAT-022`, `QA-006` → `DONE`; `KV-DAT-020` → `WIP` until the cluster configuration fixes the count at bootstrap.
- 2026-09-23 — Error model built in `internal/kverr` (ADR-0011): 14 kinds (`KindCanceled` added to cover the edge's `context.Canceled` mapping), 21 registered reasons, unexported fields with copying accessors, `New`/`Newf`/`Wrap`, `With*` copy methods, `ReasonOf`, six sentinels. Maintainer decisions: unexported fields; an empty message defaults to the reason's description; `RetryAfter` is set by the caller, with no per-reason defaults. Authentication reasons are deliberately not retryable (a new credential makes a different request). `docs/errors.md` published and pinned to the registry by `TestErrorCatalogMatchesRegistry_KV_API_092`. `KV-API-092` → `DONE`; `KV-API-090` and `KV-API-091` → `WIP` until the server's gRPC edge attaches `ErrorInfo` and `RetryInfo`. Handbook `error-handling.md` updated to match; its duplicate reason table removed in favor of the catalog. `stretchr/testify` added (on the allowlist).
- 2026-09-22 — `KV-API-002`, `KV-API-003`, `KV-API-082` set to `DONE`: each is enforced automatically by the CI `Proto` job (`proto-check`, `buf breaking`). `KV-API-080` and `KV-API-081` stay `SPEC`: the contract satisfies them, but nothing verifies them automatically yet — `KV-API-080` also needs the server to treat an unspecified consistency mode as `LINEARIZABLE`. A protoreflect-based contract test over the generated descriptors would make both verifiable.
- 2026-09-22 — `buf` tooling added: `buf.yaml` (STANDARD + COMMENTS lint, FILE breaking) and `buf.gen.yaml` at the repository root (maintainer's decision; spec §22 updated), `buf` and both plugins pinned as `go.mod` tool directives, `make proto` / `proto-lint` / `proto-breaking` / `proto-check`, and a CI `Proto` job. Generation verified in a scratch copy: builds, vets, and regenerates identically. Add `Proto` to the required checks of the `main` ruleset once merged.
- 2026-09-22 — Wire contract written in `proto/gokvx/v1/` (five files, all phases); compiles and passes `buf` STANDARD lint. Maintainer accepted deviations from Appendix A: separate `TxnService`, `Service` suffix on all services, RPC-named request and response messages, `_seconds` / `_bytes` unit suffixes, reserved field names. Appendix A updated to match. Revision semantics clarified: only commands that write at least one key consume a revision (`KV-DAT-001`); the current revision is persisted explicitly (new `KV-DAT-008`). Spec bumped to 1.1.0. Document–code divergence rule added to `AGENTS.md` §3.
- 2026-09-22 — Working agreement and handbook merged (#3). Follow-up: the project is described by what it does rather than as a "reference implementation", and the PR checklist separates items for every change from items for requirement work.
- 2026-09-22 — Repository published. CI, pre-commit hooks, branch ruleset on `main`, CodeQL, Dependabot, and private vulnerability reporting are active. Working agreement and handbook drafted on `docs/engineering-guidelines`.
