# Go Standards

How Go code in gokvx is written. The baseline is [Effective Go](https://go.dev/doc/effective_go), the [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), and the [Google Go Style Guide](https://google.github.io/styleguide/go/); this document records the project-specific decisions on top of them. Formatting and a large part of this document are enforced by `golangci-lint` ([.golangci.yml](../../.golangci.yml)); the rest is enforced in review.

---

## 1. Principles

1. **Clarity over cleverness.** Code is read far more often than it is written. Prefer the obvious solution, even when it is longer.
2. **The specification drives the code.** Types, names, and behavior follow `docs/requirements.md`. When code and spec disagree, the code is wrong or the spec needs an explicit amendment.
3. **Explicit over implicit.** Dependencies are passed in, not reached for. Behavior is configured, not inferred.
4. **Small, focused units.** A package has one responsibility; a function does one thing.
5. **Make invalid states unrepresentable.** Use types, `oneof`, and constructors so that invalid combinations cannot be built, rather than checking for them everywhere.

## 2. Package layout and dependencies

- `cmd/<binary>` contains only wiring: parse configuration, construct components, run, and shut down. No business logic.
- `internal/<package>` holds the implementation. Package names are short, lower-case, singular nouns (`storage`, `mvcc`, `shard`) — never `util`, `common`, `helpers`, or `misc`.
- `pkg/client` is the only public Go API. It depends on generated code and the standard library, never on `internal/` implementation packages.
- Generated code lives in `gen/`, is produced only by `make proto`, is committed, and is never edited by hand. CI fails if it is out of date.
- Layering (spec §6.2) is one-directional:

```text
server (transport) → service handlers → shard (routing) → consensus / command log → mvcc → storage
                                                          (never the reverse)
```

- The replication layer (`consensus`, command log) never imports `server` (A-3). The service layer never imports `storage` directly (A-1).
- Interfaces are defined in the package that **uses** them, not the one that implements them, and are kept small — usually one to three methods.
- An import cycle is a design error to be fixed by restructuring, never by moving code into a shared "common" package.

## 3. Naming

- Follow Go conventions: `MixedCaps`, initialisms in consistent case (`ID`, `URL`, `TLS`, `JWKS`, `RPC`), no `Get` prefix on getters.
- Use the spec's vocabulary exactly: `Revision`, `ModRevision`, `CreateRevision`, `Version`, `Slot`, `ShardGroup`, `Lease`, `Compact`. Never invent a synonym.
- Names describe meaning, not type: `appliedIndex`, not `idxUint64`.
- Receiver names are short and consistent across a type's methods (`s *Store`, `l *Log`).
- Error reasons are `UPPER_SNAKE_CASE` strings (`KEY_TOO_LARGE`); Go identifiers for them are `ReasonKeyTooLarge`.
- Test names follow [testing.md §3](testing.md#3-naming-and-traceability).

## 4. Comments and documentation

Comments exist so that a reader who has never seen the code understands **what** each part does and **why** it is built that way.

### Required comments

| Element | Rule |
|---|---|
| Package | Every package has a `doc.go` with a package comment: its responsibility, its key invariants, and the spec sections it implements. |
| Function or method | Every function and method — exported or not — has a doc comment of one to three lines starting with its name and stating **what it does**, not how. |
| Exported type, constant, variable | A doc comment starting with its name. |
| Struct field | A comment when its meaning, unit, or invariant is not obvious from its name and type (`// in bytes`, `// guarded by mu`, `// zero means no lease`). |
| Concurrency | Every type states whether it is safe for concurrent use. Every mutex documents what it guards. |
| Spec-driven behavior | A comment citing the requirement ID: `// KV-STO-004: a checksum failure mid-log is fatal.` |

Test functions are exempt when their name states the behavior under test; the `// Verifies:` line is still required (see testing.md).

### Style

- Complete sentences, starting with the identifier's name and ending with a period (`godot` enforces the period).
- Explain **why** inside function bodies; the code already says what. A comment restating the code is noise and is removed.
- Document behavior at the edges: what happens with `nil`, empty input, cancellation, and errors.
- Keep comments true. A change that makes a comment wrong updates the comment in the same commit.

### Not allowed

- Commented-out code — version control remembers it.
- `TODO` or `FIXME` without an issue: the only accepted form is `// TODO(#123): what remains and why`.
- Author names, dates, or change history in comments.
- Decorative banners or ASCII art in Go files.

### Example

```go
// Append writes rec to the active segment and returns its log index.
// The record is durable before Append returns when the fsync policy is
// "always" (KV-STO-005). Append is safe for concurrent use.
func (l *Log) Append(ctx context.Context, rec Record) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Rotate before writing so a record never spans two segments; recovery
	// relies on each segment being independently readable (KV-STO-003).
	if l.active.size+rec.encodedLen() > l.cfg.SegmentBytes {
		if err := l.rotate(ctx); err != nil {
			return 0, fmt.Errorf("storage.Log.Append: %w", err)
		}
	}
	// ...
}
```

## 5. Size and complexity

Limits keep every unit small enough to understand in one reading. The "lint" column is enforced by `golangci-lint`; "target" is what code should normally look like.

| Measure | Target | Limit | Enforced by |
|---|---|---|---|
| Function length | ≤ 40 lines | 80 lines / 50 statements | `funlen` |
| Cognitive complexity | ≤ 10 | 20 | `gocognit` |
| Nested `if` depth | ≤ 2 | complexity 5 | `nestif` |
| Parameters (excluding `ctx`) | ≤ 3 | 5 — beyond that, a config struct | review |
| File length (excluding tests, generated code) | ≤ 400 lines | 600 lines | review |
| Interface size | 1–3 methods | 5 methods | review |
| Line length | ≤ 100 characters | 120 characters | review |

Tests are exempt from function length and complexity limits, but a test that is hard to read is split.

**How to stay inside the limits:** return early instead of nesting; extract a well-named helper instead of a comment explaining a block; split a file by responsibility (`log_append.go`, `log_recovery.go`), not alphabetically.

## 6. Constructors and configuration

### Constructors

- Components are built with `New<Type>(cfg Config, deps...) (*Type, error)`:
  - `Config` is a plain struct of settings with a `Validate() error` method, called first thing in the constructor.
  - Required collaborators (a `Clock`, an `Engine`, a logger) are explicit parameters, never optional.
  - The constructor returns an error rather than panicking on invalid input.
- Zero values are safe or impossible: if a zero `Config` field has no sensible default, `Validate` rejects it.
- **Functional options are used only in the public client SDK** (`pkg/client`), where they let the API grow without breaking callers: `client.New(endpoints, client.WithTLS(cfg), client.WithTimeout(d))`. Internal packages use config structs, which are easier to validate and to read in one place.

### Application configuration

- Loaded with `koanf` from YAML file, `GOKVX_*` environment variables, and command-line flags, in that order of increasing precedence (`KV-CFG-001`).
- Decoded into typed structs with **unknown keys rejected** (`KV-CFG-020`).
- Every configuration section has a hand-written `Validate() error` that checks ranges, cross-field constraints, and safety rules, and names the offending key in its error (`KV-CFG-002`) — for example `cluster.raft.election_ticks: must be at least 10 × heartbeat_ticks (got 5)`.
- Validation collects **all** violations before failing, so an operator fixes a file in one pass.
- There is no global configuration object. Each component receives only its own section.

## 7. Context and cancellation

- `ctx context.Context` is the first parameter of any function that performs I/O, blocks, waits, or crosses a package boundary. It is never stored in a struct.
- Deadlines and cancellation are honored promptly: long loops check `ctx.Done()`; blocking operations select on it.
- A canceled request never leaves work without an owner — for example a Raft proposal whose caller has gone is still tracked until it commits or is abandoned explicitly (`KV-API-007`).
- `context.Background()` appears only in `main`, tests, and long-lived background loops started at construction.
- Trace context travels in `ctx`; request-scoped values (principal, request ID) use unexported, typed context keys with accessor functions.

## 8. Concurrency

- **Every goroutine has an owner** that starts it, can stop it, and waits for it to finish. Use `errgroup` or an explicit `sync.WaitGroup` plus a stop channel or context.
- A component with background work exposes `Run(ctx) error` or `Start`/`Close`, and `Close` does not return until its goroutines have exited.
- Channels are bounded. An unbounded queue is a memory leak waiting for a slow consumer (`KV-API-045`).
- Shared state is protected by a mutex whose comment names the fields it guards. Prefer a mutex to atomics unless profiling shows contention.
- Never hold a lock while doing I/O, calling a callback, or sending on a channel that may block.
- Package tests that start goroutines verify leaks with `goleak` (testing.md §6).

## 9. Determinism, time, and randomness

- Time is read through an injected `clock.Clock` (`internal/clock`); tests drive it with `clocktest.Fake`. Randomness comes from an injected source.
- The state machine never reads the clock or a random source at all (A-2): time-dependent values arrive inside commands.
- Never iterate a map where the order can influence output, persisted state, or the wire. Sort keys first.

## 10. Errors

Covered in full by [error-handling.md](error-handling.md). In short: one error type (`*kverr.Error`), functions return `error`, wrap with `%w` at boundaries, log once at the edge.

## 11. Logging, metrics, and tracing

- Logging uses `log/slog` with JSON output, built by `observability.NewLogger`. Loggers are passed in, never taken from a global. Request-scoped fields travel in the context via `observability.WithAttrs` and appear on every record logged with that context; use the `*Context` logging methods.
- A `config.Config` logs as its redacted form, one attribute per key, so logging a configuration can never leak credentials.
- Attribute keys are `snake_case` and consistent with spec §12.3 (`trace_id`, `request_id`, `principal`, `method`, `node_id`).
- Levels: `DEBUG` per request; `INFO` lifecycle and state changes only; `WARN` degraded but serving; `ERROR` needs attention. Log volume must not scale with request volume at `INFO` (`KV-OBS-023`).
- Metric names, types, and labels come verbatim from spec Appendix C. A new metric is added to Appendix C in the same PR.
- Keys, values, tokens, and secrets never appear in logs, span attributes, or labels. Keys are represented by a hash of their prefix when needed (`KV-OBS-004`).

## 12. Security in code

- Compare secret material with `crypto/subtle.ConstantTimeCompare`.
- Never build TLS configuration ad hoc — use the shared constructor that enforces TLS 1.3 and client-certificate verification.
- Validate all input at the boundary where it enters the system (size, emptiness, `oneof` presence) before any other work.
- Bound every resource an external caller can consume: request size, stream count, watch count, buffer length, pagination limit.
- `gosec` findings are fixed, not suppressed. A justified exception carries `//nolint:gosec // <reason>` on the exact line.

## 13. Performance

- Correctness and clarity first; optimize only with a benchmark or profile that shows the need, and keep the benchmark.
- Avoid allocation in hot paths identified by profiling (encoding, the apply loop, watch fan-out) — not speculatively.
- Pre-size slices and maps when the size is known.
- Latency claims are measured with the methodology in spec §18.5, never estimated.

## 14. Lint suppressions

`//nolint` is allowed only for a specific linter on a specific line with a reason: `//nolint:gosec // G304: path comes from validated configuration`. File-wide or blanket suppressions are not allowed.
