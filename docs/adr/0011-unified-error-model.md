# 0011. One error type with a registered reason, translated only at the edges

- **Status:** Accepted
- **Date:** 2026-09-23
- **Requirements:** `KV-API-090`, `KV-API-091`, `KV-API-092`, layering rule A-5, `KV-SEC-030`, `MS1-API-007`, `MS1-API-008`

## Context and problem

gokvx reports failures from every layer — storage, MVCC, consensus, authentication, transport — to clients over gRPC. microservice-1 receives those failures and must turn them into HTTP responses. The specification constrains how:

- Every client-visible error carries a gRPC code and a `google.rpc.ErrorInfo` with a `reason` from a fixed, documented enumeration; retryable errors carry `RetryInfo` (`KV-API-090`–`092`).
- Only the transport layer converts errors to gRPC status codes; every other layer wraps errors with `%w` and carries a typed identifier (A-5).
- Authentication failures must not reveal which check failed (`KV-SEC-030`).
- microservice-1 maps the same gRPC code to different HTTP statuses depending on the reason — `PERMISSION_DENIED` from gokvx becomes `502`, while a namespace violation is `403` (Appendix B.2).

The question is what error representation lets every layer report failures consistently, keeps internal detail away from clients, and serves two different wire protocols.

## Decision drivers

- One vocabulary for failures across both services, logs, metrics, and tests.
- Transport independence: the storage and consensus layers must not import gRPC or HTTP.
- Safety by construction: no path by which an unclassified or internal error reaches a client unintentionally.
- Idiomatic Go: compatibility with `errors.Is`, `errors.As`, `%w`, and every library that returns `error`.
- A public reason catalog that cannot drift from the code.

## Considered options

1. **One error type with a registered reason, translated only at the edges.**
2. **A transport-specific error in each layer** — `status.Error` from gRPC in the server, sentinel errors elsewhere.
3. **An HTTP-centric application error** — a single `*AppError` carrying an HTTP status code, returned from every function.

## Decision

Chosen option: **one error type with a registered reason, translated only at the edges** (option 1), implemented as `internal/kverr`:

- `*kverr.Error` carries a `Reason` — a stable, upper-snake-case string that is part of the public contract — plus a client-safe message, optional non-sensitive metadata, field violations, a retry hint, and an optional cause.
- A single registry maps each reason to its `Kind` and retryability. The kind, which decides the gRPC code, is derived from the reason, so the two can never disagree. An unregistered reason is replaced by `INTERNAL`, so only cataloged reasons reach clients.
- The type's fields are unexported: errors are built only by `New`, `Newf`, and `Wrap`, and are immutable — `With*` methods return copies. This makes sentinel errors genuinely constant.
- Functions return the `error` interface, never `*kverr.Error`, which avoids the typed-nil trap.
- Only the gRPC edge in gokvx and the HTTP edge in microservice-1 translate errors into wire formats, each through one function covered by a table-driven test.
- The public catalog in `docs/errors.md` is checked against the registry by an automated test.

## Consequences

- **Positive:** Every failure speaks the same vocabulary end to end. Lower layers stay free of transport imports. Internal detail cannot leak through a client message, because messages and causes are separate fields with separate audiences. The catalog is verifiably accurate.
- **Negative:** Every new failure mode requires a registry entry, a catalog row, and a reason name that is permanent once released. Accessor methods are slightly more verbose than reading fields directly.
- **Follow-up:** The gRPC edge translation arrives with the server; the HTTP edge and microservice-1's own reasons arrive with that service; a `slog.LogValuer` arrives with the logging package; the public SDK in `pkg/client` needs its own exported error type carrying the same reasons.

## Options in detail

### Option 1: One error type with a registered reason

Satisfies every driver. The cost is the discipline of registering reasons, which the registry and catalog tests make mechanical.

### Option 2: Transport-specific errors per layer

gRPC status errors are convenient in the server, but they would either leak into lower layers — violating A-5 and tying storage to gRPC — or require each layer to invent its own sentinels, producing several vocabularies and translation code between each pair of layers. microservice-1 would receive only codes and messages, with nothing stable to distinguish reasons that share a code.

### Option 3: HTTP-centric application error

A pattern that works well for a single REST service. For gokvx it fails three drivers:

- gokvx speaks only gRPC, so an HTTP status stored in the error is meaningless there, and the same error must become different HTTP statuses in microservice-1 depending on its reason.
- Returning a concrete `*AppError` from every function invites the typed-nil bug and breaks compatibility with code that expects `error`.
- A hand-built "internal message" breadcrumb duplicates what `%w` wrapping already provides, and drifts from it.

Its good ideas are kept in option 1: a single type, constructors instead of literals, a stable machine-readable code, a client-safe message separate from internal detail, field-level validation details, and `errors.Is` comparison by code.
