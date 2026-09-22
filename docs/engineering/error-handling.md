# Error Handling

gokvx uses **one error type across the whole system**: `*kverr.Error`, defined in `internal/kverr`. It is created where a failure is detected, travels through every layer unchanged, and is translated exactly once at each edge — into a gRPC status in `gokvx`, and into an HTTP response in `microservice-1`.

Spec references: layering rule A-5 (§6.2), `KV-API-090`–`092`, Appendix B, `KV-SEC-030`.

---

## 1. Design goals

1. **One vocabulary.** Every failure in the system is described by a stable `Reason`. Clients, logs, metrics, and tests all speak it.
2. **Transport-agnostic.** The same error must become a gRPC status in gokvx and an HTTP status in microservice-1, so the error itself stores neither.
3. **Safe by construction.** What a client may see (`Message`) is kept apart from what only operators may see (the cause chain), so internal details cannot leak by accident (`KV-SEC-030`).
4. **Idiomatic Go.** Works with `errors.Is`, `errors.As`, and `%w` wrapping, and with every library that returns `error`.

## 2. The type

```go
// Error is the single error type used across gokvx. Construct it with New,
// Newf, or Wrap — never with a struct literal — so the reason is always set.
type Error struct {
	// Reason is the stable, machine-readable identifier sent to clients as
	// google.rpc.ErrorInfo.reason. It is part of the public contract.
	Reason Reason

	// Message is the client-safe explanation. It never contains keys,
	// values, token contents, or internal detail.
	Message string

	// Metadata is non-sensitive context for ErrorInfo.metadata,
	// e.g. {"limit_bytes": "1024"} or {"leader": "gokvx-1:2379"}.
	Metadata map[string]string

	// Violations lists field-level validation failures; it is sent as
	// google.rpc.BadRequest and only used with invalid-argument reasons.
	Violations []FieldViolation

	// RetryAfter is the suggested backoff for retryable reasons; it is sent
	// as google.rpc.RetryInfo. Zero means "use the client's default".
	RetryAfter time.Duration

	// Err is the underlying cause. It is logged and traced, never sent to
	// clients, and is reachable through errors.Unwrap.
	Err error
}
```

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
)
```

Illustrative excerpt of the registry — the full list is the public catalogue in `docs/errors.md` (`KV-API-092`):

| Reason | Kind | Retryable | Raised when |
|---|---|---|---|
| `KEY_EMPTY` | InvalidArgument | no | An empty key outside a range boundary (`KV-DAT-012`) |
| `KEY_TOO_LARGE` | InvalidArgument | no | Key exceeds `limits.max_key_bytes` |
| `VALUE_TOO_LARGE` | InvalidArgument | no | Value exceeds `limits.max_value_bytes` |
| `REVISION_COMPACTED` | OutOfRange | no | Read or scan below the compaction point |
| `MISSING_CREDENTIAL` | Unauthenticated | after refresh | No or malformed `authorization` metadata |
| `INVALID_TOKEN` | Unauthenticated | after refresh | Signature or claims invalid |
| `UNKNOWN_KEY_ID` | Unauthenticated | after refresh | `kid` not in JWKS after the permitted refresh |
| `INSUFFICIENT_SCOPE` | PermissionDenied | no | Token lacks the method's scope |
| `RATE_LIMITED` | ResourceExhausted | yes | Per-principal rate limit exceeded |
| `QUOTA_EXCEEDED` | ResourceExhausted | yes | Database size quota exceeded |
| `WATCH_BUFFER_OVERFLOW` | ResourceExhausted | yes | Slow watch consumer (`KV-API-045`) |
| `STALE_READ_BOUND_UNMET` | FailedPrecondition | yes | `min_revision` above the local applied revision |
| `NOT_LEADER` | FailedPrecondition | yes | Write on a follower with forwarding disabled |
| `CROSS_SHARD_TXN_UNSUPPORTED` | FailedPrecondition | no | Transaction spans shard groups |
| `FEATURE_NOT_IN_CURRENT_PHASE` | Unimplemented | no | Field or RPC not functional yet (`KV-API-000`) |
| `NO_LEADER` | Unavailable | yes | No leader known within the wait window |
| `SHUTTING_DOWN` | Unavailable | yes | Node is draining |
| `INTERNAL` | Internal | no | Invariant violation or unexpected failure |

### Construction

```go
// Fields are validated at the edge and fail with a client-safe message.
return kverr.New(kverr.ReasonKeyTooLarge, "key exceeds the maximum length").
	WithMetadata("limit_bytes", strconv.Itoa(cfg.MaxKeyBytes))

// A foreign error enters the system: wrap it once, with the reason that
// describes it from gokvx's point of view.
if err := l.file.Sync(); err != nil {
	return kverr.Wrap(kverr.ReasonInternal, err, "command log sync failed")
}
```

- `New`, `Newf`, and `Wrap` are the only constructors. `With*` methods return a modified **copy**; an `Error` is never mutated after it is created.
- A foreign error (from `os`, Pebble, the network) is wrapped **once**, at the boundary where it enters gokvx code.

### Inspection

```go
if kverr.ReasonOf(err) == kverr.ReasonRevisionCompacted { ... }   // returns ReasonInternal for non-kverr errors
if errors.Is(err, kverr.ErrNotLeader) { ... }                      // sentinels compare by Reason
var kerr *kverr.Error
if errors.As(err, &kerr) && kerr.Kind() == kverr.KindUnavailable { ... }
```

Error strings are never compared.

## 3. Rules

1. **Return `error`, never `*kverr.Error`.** A function that returns a concrete error pointer creates the typed-nil trap: a nil `*kverr.Error` assigned to an `error` is not `nil`, and `if err != nil` silently becomes true. Returning the `error` interface is the Go convention and keeps every function compatible with the standard library. Callers that need the details use `errors.As` or `kverr.ReasonOf`.
2. **Create at the origin.** The layer that detects a condition creates the `*kverr.Error`, because only it knows the correct reason.
3. **Wrap on the way up.** Intermediate layers add context without changing the reason: `return fmt.Errorf("mvcc.Put: %w", err)`. The prefix is `package.Function` so the chain reads as a trace: `server.Put: mvcc.Put: storage.Log.Append: INTERNAL: command log sync failed: fsync /data/0007.log: input/output error`.
4. **Translate only at the edge.** Only the gRPC transport layer (gokvx) and the HTTP layer (microservice-1) convert errors into wire formats (A-5).
5. **Log once, at the edge.** A function either handles an error or returns it — never both. The edge logs it with the full cause chain and the request's trace context.
6. **No panics across boundaries.** A panic is a programmer error. The recovery interceptor converts it into `INTERNAL`, logs the stack at `ERROR`, and increments a metric.
7. **Absence is not an error.** A missing key is `count = 0`; a failed comparison is `succeeded = false` (`KV-API-010`, `KV-API-033`).
8. **Messages are safe.** Authentication and authorization messages are generic and never reveal which check failed or whether a key exists; the specific cause goes into `Err` for the logs (`KV-SEC-030`).

## 4. The gRPC edge (gokvx)

`internal/server` converts an error into a `*status.Status` in one function, covered by a table-driven test over every registered reason:

| Source | Result |
|---|---|
| `*kverr.Error` | gRPC code from its kind; message from `Message`; `ErrorInfo{Reason, Domain: "gokvx.io", Metadata}`; `RetryInfo` if retryable; `BadRequest` if it has violations |
| `context.Canceled` | `CANCELED` |
| `context.DeadlineExceeded` | `DEADLINE_EXCEEDED` with reason `DEADLINE_EXCEEDED` |
| anything else | `INTERNAL` with a generic message, logged at `ERROR` — an unclassified error reaching the edge is itself a bug to fix |

Log levels at the edge: `KindInternal` → `ERROR`; `KindUnavailable` and `KindDeadlineExceeded` → `WARN`; client-caused kinds → `DEBUG`, since they are counted by `gokvx_grpc_requests_total{code}` and must not make log volume scale with misbehaving clients. Authentication and authorization failures additionally go to the security audit log (`KV-SEC-035`).

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
| kind `ResourceExhausted` | `429` with `Retry-After` from `RetryAfter` |
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

A common pattern stores an HTTP status code inside the application error and returns `*AppError` from every function. It works for a single REST service, but not for gokvx:

- gokvx speaks gRPC only; an HTTP status inside its errors would be meaningless there, and the same error must become different HTTP statuses in microservice-1 depending on its reason.
- Returning a concrete error pointer invites the typed-nil bug and breaks compatibility with code that expects `error`.
- A hand-built "internal message" breadcrumb duplicates what `%w` wrapping already provides, and drifts from it.

The ideas worth keeping from that pattern are kept: a single type, constructors instead of literals, a stable machine-readable code, a client-safe message separate from internal detail, field-level validation details, and `errors.Is` comparison by code.
