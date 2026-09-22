# Dependencies

Every third-party module is a long-term commitment: code we run but did not write, which must be kept secure and up to date. gokvx therefore depends on a deliberately small set of widely used, actively maintained modules, each chosen for a stated reason.

---

## 1. Policy

1. **Standard library first.** A dependency is added only when the standard library cannot do the job reasonably.
2. **Allowlist only.** Production and test code import only the modules listed in §2. Adding a module requires an accepted ADR in `docs/adr/` that states the need, the alternatives considered, and the maintenance and security posture of the module.
3. **Major versions are decisions.** Upgrading a major version, or replacing a module, also requires an ADR. Minor and patch updates arrive through Dependabot and are merged when CI passes.
4. **Versions live in `go.mod`.** This document names modules, not versions. Tool versions are pinned in the `Makefile` and `.github/workflows/ci.yml` together.
5. **Every dependency is scanned.** `govulncheck` runs on every pull request and daily (`QA-041`).

## 2. Allowlist

### Runtime

| Area | Module | Used for |
|---|---|---|
| RPC | `google.golang.org/grpc` | gRPC server and client, health service, reflection in development mode |
| RPC | `google.golang.org/protobuf` | Protocol Buffer runtime |
| RPC | `google.golang.org/genproto/googleapis/rpc` | `google.rpc.Status`, `ErrorInfo`, `RetryInfo`, `BadRequest` error details |
| Storage | `github.com/cockroachdb/pebble/v2` | Embedded LSM storage engine (`KV-STO-001`, `ADR-0005`) |
| Consensus | `go.etcd.io/raft/v3` | Raft algorithm; storage, transport, and apply loop are ours (`KV-CON-001`, `ADR-0002`) |
| Security | `github.com/golang-jwt/jwt/v5` | JWT parsing and standard claim validation; the JWKS cache and refresh logic are ours, because the spec defines their behaviour precisely (`KV-SEC-012`–`019`) |
| Security | `github.com/fsnotify/fsnotify` | Watching certificate files for hot reload (`KV-SEC-005`) |
| Resilience | `golang.org/x/time` | Token-bucket rate limiting (`KV-SEC-032`) |
| Resilience | `golang.org/x/sync` | `errgroup` for goroutine lifecycles; `singleflight` for token refresh (`MS1-SEC-005`) |
| Resilience | `github.com/sony/gobreaker/v2` | Circuit breakers in microservice-1 (`MS1-SEC-006`, `MS1-API-026`) |
| Caching | `github.com/hashicorp/golang-lru/v2` | Bounded, expiring verified-claims cache (`KV-SEC-017`) |
| Configuration | `github.com/knadh/koanf/v2` with its `file`, `env`, `posflag` providers and `yaml` parser | Layered configuration: file, environment, flags (`KV-CFG-001`) |
| CLI | `github.com/spf13/cobra` | Command structure and shell completion for `gokvx`, `gokvxctl`, `kvbench`, `kvcheck` |
| Observability | `go.opentelemetry.io/otel` and its SDK, OTLP exporter, and `otelgrpc` / `otelhttp` instrumentation | Distributed tracing (`KV-OBS-001`–`007`) |
| Observability | `github.com/prometheus/client_golang` | Metrics and exemplars (`KV-OBS-010`–`015`) |
| TUI | `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2` | Cluster dashboard in `gokvxctl` (`KV-OPS-001`–`007`) |
| Benchmarking | `github.com/HdrHistogram/hdrhistogram-go` | Accurate latency percentiles in `kvbench` (`QA-031`, `QA-034`) |

Standard library packages used where others often reach for a dependency: `log/slog` for logging, `net/http` with its pattern-matching `ServeMux` for the microservice-1 REST API, `hash/crc32` (Castagnoli) for checksums and slots, `crypto/rand` for identifiers.

### Test only

| Module | Used for |
|---|---|
| `github.com/stretchr/testify` | Assertions — `require` and `assert` packages only |
| `pgregory.net/rapid` | Property-based tests (`QA-004`) |
| `go.uber.org/goleak` | Goroutine leak detection |
| `github.com/testcontainers/testcontainers-go` | Multi-node integration tests (`QA-011`) |
| `github.com/anishathalye/porcupine` | Linearizability checking (`QA-020`) |

### Build tools

| Tool | Pinned in | Used for |
|---|---|---|
| `buf` | `tools/go.mod` `tool` directive | Protocol Buffer linting, breaking-change detection, and generation. It lives in its own module so its large dependency tree stays out of the main one. |
| `protoc-gen-go`, `protoc-gen-go-grpc` | `go.mod` `tool` directives | Code generation. Pinned in the main module so their versions track the protobuf and gRPC runtimes. |
| `golangci-lint` | `Makefile`, CI | Linting and formatting |
| `govulncheck` | `Makefile`, CI | Vulnerability scanning |
| `gitleaks` | `Makefile`, CI, pre-commit | Secret scanning |
| `pre-commit` | `Makefile` | Git hook management |

## 3. Technology decisions

The choices below were made when the project started. Each records what was chosen, why, and what was rejected.

### CLI framework: Cobra

**Chosen:** `spf13/cobra` for every binary.
**Why:** The CLI needs nested subcommands (`gokvxctl member add`, `gokvx config validate`) and shell completion for bash, zsh, and fish (`KV-OPS-013`), which Cobra generates. It is the de facto standard for infrastructure CLIs — `kubectl`, `etcdctl`, `helm`, and `gh` all use it — so the interface is immediately familiar to operators.
**Rejected:** The standard library `flag` package — it has no subcommands or completion, so both would be hand-built and maintained.

### Configuration: koanf with hand-written validation

**Chosen:** `knadh/koanf/v2` to merge file, environment, and flag sources; typed structs with a `Validate()` method per section.
**Why:** The spec requires strict precedence (`KV-CFG-001`) and the rejection of unknown keys (`KV-CFG-020`). koanf is modular, has no global state, preserves key case, and decodes with unknown keys treated as errors. Validation is explicit Go code so every rule is visible and testable, and every error names its key.
**Rejected:** Viper — global state by default, forced lower-casing of keys, a large transitive dependency tree, and no clean way to reject unknown keys. Hand-written loading — reimplements merging and environment mapping for no benefit.

### Assertions: testify

**Chosen:** `stretchr/testify`, `require` and `assert` only.
**Why:** The most widely used Go assertion library; its failure output shows full diffs, which makes failures quick to diagnose.
**Rejected:** `testify/mock` and `testify/suite` — see test doubles and plain tests below.

### Property testing: rapid

**Chosen:** `pgregory.net/rapid`.
**Why:** Typed generators, automatic shrinking to a minimal failing case, and a design suited to state-machine testing — exactly what the MVCC invariants need (`QA-004`).
**Rejected:** `testing/quick` — frozen by the Go team, and it does not shrink failing inputs.

### Constructors: config structs internally, functional options publicly

**Chosen:** `New<Type>(cfg Config, deps...)` with `cfg.Validate()` for internal components; functional options only in the public SDK `pkg/client`.
**Why:** Config structs keep every setting visible in one place and validate as a unit, which suits components configured from files. Functional options suit a public API, where new options can be added without breaking callers.
**Rejected:** Functional options everywhere — they scatter configuration across many small functions and make validation of combined settings awkward.

### Test doubles: hand-written fakes

**Chosen:** Small, hand-written fakes of gokvx's own interfaces, in `<package>test` packages, verified by the same contract tests as the real implementations.
**Why:** Fakes behave like the real thing, keep tests readable, and do not break when internal call order changes. This follows the Go community's and Google's guidance, and interfaces kept small (go-standards.md §2) make fakes cheap to write.
**Rejected:** Generated mocks (`mockery`, `gomock`, `testify/mock`) — they couple tests to call sequences rather than behaviour, and generated code adds noise to every change.

### Protocol Buffers: `buf` with local plugins

**Chosen:** `buf` for linting, breaking-change detection, and generation, with `protoc-gen-go` and `protoc-gen-go-grpc` run locally. All three are pinned as `tool` directives and run through `go tool`: the plugins in `go.mod`, so their versions stay aligned with the protobuf and gRPC runtimes, and `buf` in a separate `tools/go.mod`, so its roughly ninety transitive dependencies never enter the main module's graph.
**Why:** Generation is hermetic and reproducible — the same versions on every machine and in CI, recorded in `go.mod`, updated by Dependabot, and with no dependency on an external service at build time.
**Rejected:** Buf Schema Registry remote plugins — generation would depend on network access and a third-party service, and plugin versions would live outside `go.mod`. Raw `protoc` — needs a separately installed binary and offers no built-in lint or breaking-change checks.

### Logging: `log/slog`

**Chosen:** The standard library's structured logger.
**Why:** Structured, fast, JSON output, and part of the standard library since Go 1.21 — no dependency needed (`KV-OBS-020`).
**Rejected:** `zap`, `zerolog`, `logrus` — no longer necessary.

### HTTP routing: standard library

**Chosen:** `net/http` `ServeMux` with method and wildcard patterns (`PUT /api/v1/kv/{key...}`).
**Why:** Since Go 1.22 it supports everything microservice-1 needs, including the trailing wildcard that lets keys contain slashes (`MS1-API-001`).
**Rejected:** `chi`, `gin`, `echo` — an extra dependency for no remaining benefit.
