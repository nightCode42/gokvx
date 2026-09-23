# Error Handling

gokvx uses **one error type across the whole system**: `*kverr.Error`, defined in `internal/kverr`. It is created where a failure is detected, travels through every layer unchanged, and is translated exactly once at each edge — into a gRPC status in `gokvx`, and into an HTTP response in `microservice-1`.

Spec references: layering rule A-5 (§6.2), `KV-API-090`–`092`, Appendix B, `KV-SEC-030`. Decision record: [ADR-0011](../adr/0011-unified-error-model.md). Public catalog: [docs/errors.md](../errors.md).

---

## 1. Design goals

1. **One vocabulary.** Every failure in the system is described by a stable `Reason`. Clients, logs, metrics, and tests all speak it.
2. **Transport-agnostic.** The same error must become a gRPC status in gokvx and an HTTP status in microservice-1, so the error itself stores neither.
3. **Safe by construction.** What a client may see (the message) is kept apart from what only operators may see (the cause chain), so internal details cannot leak by accident (`KV-SEC-030`).
4. **Idiomatic Go.** Works with `errors.Is`, `errors.As`, and `%w` wrapping, and with every library that returns `error`.

## 2. The type

`*kverr.Error` has **unexported fields**, read through accessor methods:

| Accessor | Content | Seen by clients |
|---|---|---|
| `Reason()` | The stable identifier, sent as `google.rpc.ErrorInfo.reason`; always a registered reason | yes |
| `Kind()`, `Retryable()` | Derived from the reason through the registry | as the gRPC code and `RetryInfo` |
| `Message()` | The client-safe explanation; never empty — defaults to the reason's description | yes |
| `Metadata()` | Non-sensitive context for `ErrorInfo.metadata`, lower-camel-case keys naming their unit (`limitBytes`) | yes |
| `Violations()` | Field-level failures for `google.rpc.BadRequest` | yes |
| `RetryAfter()` | Suggested backoff for `RetryInfo`; zero means the client's own default | yes, when retryable |
| `Unwrap()` | The underlying cause | **no** — logs and traces only |
| `Error()` | Reason, message, and the cause chain | **no** — logs and traces only |

Unexported fields make two properties guarantees rather than conventions: an error can only be built by the constructors, so its reason is always registered and its message always set; and it can never change after creation — the `With*` methods return copies and the accessors return copies of the map and slice. An `Error` is therefore safe to share between goroutines and to keep in a package-level sentinel.

### Reasons and kinds

A `Reason` is registered once, in a single table, together with its `Kind` and whether it is retryable. The kind is **derived** from the reason, so a reason can never be paired with the wrong status code.

```go
// Kind classifies an error by how a caller should react. Each kind maps to
// exactly one gRPC code at the transport edge. The zero value is KindInternal,
// so an unclassified error is never mistaken for a client error.
type Kind uint8

const (
	KindInternal Kind = iota
	KindInvalidArgument
	KindNotFound
	KindAlreadyExists
	KindPermissionDenied
	KindUnauthenticated
	KindResourceExhausted
	KindFailedPrecondition
	KindAborted
	KindOutOfRange
	KindUnimplemented
	KindUnavailable
	KindDeadlineExceeded
	KindCanceled
)
```

The registry is the single source of truth for every reason's kind, retryability, and description. The complete list is the public catalog in [docs/errors.md](../errors.md) (`KV-API-092`), which an automated test keeps identical to the registry; it is not repeated here.

**Retryable** means that repeating the same request *unchanged*, after a delay, may succeed (`KV-API-091`). Authentication failures are therefore not retryable: the request needs a new credential, which makes it a different request. microservice-1's refresh-and-retry-once behaviour is keyed on the `Unauthenticated` kind instead (`MS1-SEC-004`). A test enforces that only kinds where waiting can help — `ResourceExhausted`, `FailedPrecondition`, `Aborted`, `Unavailable`, `DeadlineExceeded` — contain retryable reasons.

### Construction

```go
// Fields are validated at the edge and fail with a client-safe message.
return kverr.New(kverr.ReasonKeyTooLarge, "key exceeds the maximum length").
	WithMetadata("limitBytes", strconv.Itoa(cfg.MaxKeyBytes))

// A foreign error enters the system: wrap it once, with the reason that
// describes it from gokvx's point of view.
if err := l.file.Sync(); err != nil {
	return kverr.Wrap(kverr.ReasonInternal, err, "command log sync failed")
}
```

- `New`, `Newf`, and `Wrap` are the only constructors. `With*` methods return a modified **copy**; an `Error` is never mutated after it is created.
- An empty message is replaced by the reason's registered description, so every error has one.
- A reason that is not registered is replaced by `INTERNAL`, so only reasons from the public catalog can ever reach a client (`KV-API-090`).
- A foreign error (from `os`, Pebble, the network) is wrapped **once**, at the boundary where it enters gokvx code.
- The retry hint is set by the code that knows the real wait — for example the rate limiter's next token time — with `WithRetryAfter`. There are no per-reason defaults.

### Inspection

```go
if kverr.ReasonOf(err) == kverr.ReasonRevisionCompacted { ... }
if errors.Is(err, kverr.ErrNotLeader) { ... }                      // sentinels compare by reason
var kerr *kverr.Error
if errors.As(err, &kerr) && kerr.Kind() == kverr.KindUnavailable { ... }
```

`ReasonOf` returns the reason of the first `*kverr.Error` in the chain. For an error that never passed through `kverr` it classifies what it finds: `context.Canceled` becomes `CANCELED`, `context.DeadlineExceeded` becomes `DEADLINE_EXCEEDED`, and anything else is `INTERNAL`, because an error nobody classified is a server fault. A nil error has the empty reason.

Sentinels exist for the reasons code routinely branches on: `ErrRevisionCompacted`, `ErrNotLeader`, `ErrNoLeader`, `ErrQuotaExceeded`, `ErrShuttingDown`, `ErrFeatureNotInCurrentPhase`. For any other reason, compare `ReasonOf(err)`.

Error strings are never compared.

## 3. Rules

1. **Return `error`, never `*kverr.Error`.** A function that returns a concrete error pointer creates the typed-nil trap: a nil `*kverr.Error` assigned to an `error` is not `nil`, and `if err != nil` silently becomes true. Returning the `error` interface is the Go convention and keeps every function compatible with the standard library. Callers that need the details use `errors.As` or `kverr.ReasonOf`. The constructors themselves return `*kverr.Error` so the `With*` methods can be chained; that is safe because they never return nil. As a last line of defense, the methods the `errors` and `fmt` packages call — `Error`, `Unwrap`, `Is` — tolerate a nil receiver, so a typed nil returned by mistake cannot turn error handling into a panic.
2. **Create at the origin.** The layer that detects a condition creates the `*kverr.Error`, because only it knows the correct reason.
3. **Wrap on the way up.** Intermediate layers add context without changing the reason: `return fmt.Errorf("mvcc.Put: %w", err)`. The prefix is `package.Function` so the chain reads as a trace: `server.Put: mvcc.Put: storage.Log.Append: INTERNAL: command log sync failed: fsync /data/0007.log: input/output error`.
4. **Translate only at the edge.** Only the gRPC transport layer (gokvx) and the HTTP layer (microservice-1) convert errors into wire formats (A-5).
5. **Log once, at the edge.** A function either handles an error or returns it — never both. The edge logs it with the full cause chain and the request's trace context.
6. **No panics across boundaries.** A panic is a programmer error. The recovery interceptor converts it into `INTERNAL`, logs the stack at `ERROR`, and increments a metric.
7. **Absence is not an error.** A missing key is `count = 0`; a failed comparison is `succeeded = false` (`KV-API-010`, `KV-API-033`).
8. **Messages are safe.** Authentication and authorization messages are generic and never reveal which check failed or whether a key exists; the specific cause goes into the error's cause, for the logs (`KV-SEC-030`).

## 4. The gRPC edge (gokvx)

`internal/server` converts an error into a `*status.Status` in one function, covered by a table-driven test over every registered reason:

| Source | Result |
|---|---|
| `*kverr.Error` | gRPC code from `Kind()`; message from `Message()`; `ErrorInfo{Reason(), Domain: "gokvx.io", Metadata()}`; `RetryInfo` if `Retryable()`; `BadRequest` if it has violations |
| `context.Canceled` | `CANCELED` with reason `CANCELED` |
| `context.DeadlineExceeded` | `DEADLINE_EXCEEDED` with reason `DEADLINE_EXCEEDED` |
| anything else | `INTERNAL` with a generic message, logged at `ERROR` — an unclassified error reaching the edge is itself a bug to fix |

The edge classifies non-`kverr` errors with `kverr.ReasonOf`, which implements exactly the last three rows.

Log levels at the edge: `KindInternal` → `ERROR`; `KindUnavailable` and `KindDeadlineExceeded` → `WARN`; kinds for which `Kind.IsClientFault()` is true → `DEBUG`, since they are counted by `gokvx_grpc_requests_total{code}` and must not make log volume scale with misbehaving clients (`KV-OBS-023`). Authentication and authorization failures additionally go to the security audit log (`KV-SEC-035`).

## 5. The client side and the HTTP edge (microservice-1)

- The gokvx client code converts a received `*status.Status` back into a `*kverr.Error` using the `ErrorInfo` reason, so microservice-1 reasons about gokvx failures with the same vocabulary.
- microservice-1 registers its own HTTP-only reasons in the same registry, for example `NAMESPACE_VIOLATION` (`DEM-011`) and `UNSUPPORTED_MEDIA_TYPE` (`MS1-API-003`).
- The HTTP status is chosen at the HTTP edge by one function that checks **reason-specific rules first, then falls back to the kind**. This is required by Appendix B.2, where the same gRPC code maps differently depending on the reason:

| Reason or kind | HTTP |
|---|---|
| `NAMESPACE_VIOLATION` | `403` — the caller's request is at fault |
| `INSUFFICIENT_SCOPE` (from gokvx) | `502` — *our* service account is misconfigured; never `403` |
| kind `Unauthenticated` (from gokvx) | one reactive token refresh and a single retry (`MS1-SEC-004`); if that also fails, `503` with `Retry-After` |
| `NO_LEADER`, `NOT_LEADER`, `STALE_READ_BOUND_UNMET` | `503` with `Retry-After` |
| CAS `succeeded = false` (not an error from gokvx) | `412` via reason `PRECONDITION_FAILED` created in microservice-1 |
| kind `InvalidArgument` | `400` |
| kind `NotFound` | `404` |
| kind `ResourceExhausted` | `429` with `Retry-After` from `RetryAfter()` |
| kind `Aborted` | `409` |
| kind `OutOfRange` | `410` |
| kind `Unimplemented` | `501` |
| kind `Unavailable` | `503` with `Retry-After` |
| kind `DeadlineExceeded` | `504` |
| kind `Internal` or unknown | `500` with a generic body |

- The response body always has the shape `{"code", "message", "request_id", "details"}` (`MS1-API-007`), where `code` is the reason. The mapping is covered by a table-driven test that enumerates every gRPC code (`MS1-API-008`).

## 6. Public client SDK

`internal/kverr` is internal and cannot be imported by external users. The public SDK in `pkg/client` exposes its own exported error type carrying the same `Reason` strings, decoded from `ErrorInfo`. The reason catalogue in `docs/errors.md` is the contract both sides share.

## 7. Why not an HTTP-centric application error

The full comparison of alternatives is in [ADR-0011](../adr/0011-unified-error-model.md); the short version:

A common pattern stores an HTTP status code inside the application error and returns `*AppError` from every function. It works for a single REST service, but not for gokvx:

- gokvx speaks gRPC only; an HTTP status inside its errors would be meaningless there, and the same error must become different HTTP statuses in microservice-1 depending on its reason.
- Returning a concrete error pointer invites the typed-nil bug and breaks compatibility with code that expects `error`.
- A hand-built "internal message" breadcrumb duplicates what `%w` wrapping already provides, and drifts from it.

The ideas worth keeping from that pattern are kept: a single type, constructors instead of literals, a stable machine-readable code, a client-safe message separate from internal detail, field-level validation details, and `errors.Is` comparison by code.
