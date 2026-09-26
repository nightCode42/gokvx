# Work Log

Current focus, hand-off notes, and open decisions. This file lets any contributor — or an AI agent starting a fresh session — pick up the work exactly where it stopped.

Per-requirement implementation status is **not** tracked here; it lives only in the `Status` column of [requirements.md](requirements.md) (spec §3.3).

---

## Current focus

| Item | Value |
|---|---|
| Phase | Phase 1 — Durable core |
| Active branch | `feat/config` |
| Active work | Configuration in `internal/config` and the `gokvx` CLI — complete, awaiting the maintainer's commit and pull request |
| Requirement IDs | `KV-CFG-001`–`006`, `KV-CFG-012`, `KV-CFG-020`–`021` |

## Next up

In order. Each item is one branch and one pull request.

1. **Logging** — `internal/observability` logging: `slog` JSON handler, the standard request fields, level configuration from `observability.log`, redaction rules; a `slog.LogValuer` on `kverr.Error`. At startup, log the redacted effective configuration and each warning, which completes `KV-CFG-006` and `KV-CFG-021`. Spec §12.3.
2. **Storage engine and command log** — the `Engine` interface over Pebble and the segmented, checksummed `CommandLog`. `KV-STO-000`–`005`.
3. **Spelling** — settle on US English across the documentation, matching the linter's rule for Go code.

## Open decisions

| Decision | Owner | Notes |
|---|---|---|
| GoAuthx link in README | Maintainer | Link currently returns 404; publish GoAuthx or remove the link before sharing the repository. |

## Hand-off notes

- 2026-09-24 — Configuration built in `internal/config` with the `gokvx` CLI (`version`, `config validate`). Every key, environment variable, and flag derives from the `Config` struct, so they cannot drift; tests pin the defaults to spec Appendix D and every key to `docs/configuration.md`. Maintainer decisions: environment names use single underscores, proven collision-free by a test; one flag per key; the Appendix D values for advertise addresses, issuer and JWKS URLs, initial peers, and SAN allowlists are examples, so those keys default to empty and validation requires them where needed (Appendix D annotated, spec 1.1.1); `pre_vote`, `check_quorum`, and `default_consistency` accept only the value their MUST requirement mandates. Variables Kubernetes injects for Services (`GOKVX_*_SERVICE_HOST`, `GOKVX_*_PORT_*`) are ignored rather than rejected, which would otherwise crash every pod; the Helm chart should also set `enableServiceLinks: false`. `mapstructure` stayed indirect: unknown keys are found by comparing loaded keys with the struct, so no allowlist change beyond naming `pflag` next to Cobra. Unknown keys are reported before the value rules, because rules cannot run on unknown values. `KV-CFG-001`–`005` and `KV-CFG-020` → `DONE`; `KV-CFG-006` and `KV-CFG-021` → `WIP` until startup logging exists; `KV-CFG-012` → `WIP` until the `gokvx_build_info` metric. `KV-SEC-041`/`042` rules are enforced by validation but stay `SPEC` until a node actually starts.
- 2026-09-24 — Slot function built in `internal/shard`: `NewSlotter` validates the slot count once (zero fails with `INVALID_ARGUMENT`), `Slotter.Slot` is check-free and allocation-free, `HashInput` applies the brace rule without copying. Maintainer decisions: a positive count is a constructor precondition rather than a per-call check; a `Slotter` type instead of a bare function. The hash is anchored to published CRC32C values (check value and RFC 3720 vectors), 45 golden keys are locked in `testdata/slots.golden`, and a 45-second fuzz run (1.65M inputs) found no failures. `KV-DAT-021`, `KV-DAT-022`, `QA-006` → `DONE`; `KV-DAT-020` → `WIP` until the cluster configuration fixes the count at bootstrap.
- 2026-09-23 — Error model built in `internal/kverr` (ADR-0011): 14 kinds (`KindCanceled` added to cover the edge's `context.Canceled` mapping), 21 registered reasons, unexported fields with copying accessors, `New`/`Newf`/`Wrap`, `With*` copy methods, `ReasonOf`, six sentinels. Maintainer decisions: unexported fields; an empty message defaults to the reason's description; `RetryAfter` is set by the caller, with no per-reason defaults. Authentication reasons are deliberately not retryable (a new credential makes a different request). `docs/errors.md` published and pinned to the registry by `TestErrorCatalogMatchesRegistry_KV_API_092`. `KV-API-092` → `DONE`; `KV-API-090` and `KV-API-091` → `WIP` until the server's gRPC edge attaches `ErrorInfo` and `RetryInfo`. Handbook `error-handling.md` updated to match; its duplicate reason table removed in favor of the catalog. `stretchr/testify` added (on the allowlist).
- 2026-09-22 — `KV-API-002`, `KV-API-003`, `KV-API-082` set to `DONE`: each is enforced automatically by the CI `Proto` job (`proto-check`, `buf breaking`). `KV-API-080` and `KV-API-081` stay `SPEC`: the contract satisfies them, but nothing verifies them automatically yet — `KV-API-080` also needs the server to treat an unspecified consistency mode as `LINEARIZABLE`. A protoreflect-based contract test over the generated descriptors would make both verifiable.
- 2026-09-22 — `buf` tooling added: `buf.yaml` (STANDARD + COMMENTS lint, FILE breaking) and `buf.gen.yaml` at the repository root (maintainer's decision; spec §22 updated), `buf` and both plugins pinned as `go.mod` tool directives, `make proto` / `proto-lint` / `proto-breaking` / `proto-check`, and a CI `Proto` job. Generation verified in a scratch copy: builds, vets, and regenerates identically. Add `Proto` to the required checks of the `main` ruleset once merged.
- 2026-09-22 — Wire contract written in `proto/gokvx/v1/` (five files, all phases); compiles and passes `buf` STANDARD lint. Maintainer accepted deviations from Appendix A: separate `TxnService`, `Service` suffix on all services, RPC-named request and response messages, `_seconds` / `_bytes` unit suffixes, reserved field names. Appendix A updated to match. Revision semantics clarified: only commands that write at least one key consume a revision (`KV-DAT-001`); the current revision is persisted explicitly (new `KV-DAT-008`). Spec bumped to 1.1.0. Document–code divergence rule added to `AGENTS.md` §3.
- 2026-09-22 — Working agreement and handbook merged (#3). Follow-up: the project is described by what it does rather than as a "reference implementation", and the PR checklist separates items for every change from items for requirement work.
- 2026-09-22 — Repository published. CI, pre-commit hooks, branch ruleset on `main`, CodeQL, Dependabot, and private vulnerability reporting are active. Working agreement and handbook drafted on `docs/engineering-guidelines`.
