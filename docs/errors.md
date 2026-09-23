# Error Reference

Every error gokvx returns is a standard gRPC status with structured details, so clients can react to failures precisely without parsing text (`KV-API-090`). This page is the public catalog of those errors (`KV-API-092`).

---

## Error shape

| Part | Content |
|---|---|
| Status code | The standard gRPC code for the failure's kind, listed per reason below. |
| Message | A short, client-safe explanation. It never contains keys, values, token contents, or internal detail, and its wording may change between releases. |
| `google.rpc.ErrorInfo` | Always present. `reason` is the stable identifier from the catalog below; `domain` is `gokvx.io`; `metadata` holds non-sensitive context with lower-camel-case keys that name their unit, for example `limitBytes = "1024"`. |
| `google.rpc.RetryInfo` | Present only on retryable reasons (`KV-API-091`). `retry_delay` is the server's suggested wait, when it has one. |
| `google.rpc.BadRequest` | Present on invalid arguments that concern specific request fields; lists each violation. |

## Handling errors in a client

- **Match on `ErrorInfo.reason`, never on the message.** Reasons are stable; messages are not.
- **Retry only retryable reasons**, with exponential backoff and full jitter, and never sooner than `RetryInfo.retry_delay` when it is set.
- **On `UNAUTHENTICATED`, obtain a fresh token and retry once.** Retrying with the same token cannot succeed, which is why these reasons are not marked retryable.
- **Fall back to the status code for a reason you do not recognize.** New reasons may be added in later releases; the code always tells you the class of failure.
- **Some outcomes are not errors.** A missing key is a successful `Get` with `count = 0` (`KV-API-010`), and a comparison that does not hold is a successful `CompareAndSwap` with `succeeded = false` (`KV-API-033`).

## Stability

Reasons are part of the public contract:

- A reason is never renamed and never given a different meaning.
- New reasons may be added in any release; clients must tolerate reasons they do not know.
- A reason that falls out of use is retired, and its name is never reused.

## Catalog

"Retryable" means that repeating the same request unchanged, after a delay, may succeed.

<!-- catalog:start -->
| Reason | Status code | Retryable | Meaning |
|---|---|---|---|
| `CANCELED` | `CANCELED` | no | The caller canceled the request. |
| `CROSS_SHARD_TXN_UNSUPPORTED` | `FAILED_PRECONDITION` | no | The transaction spans more than one shard group, which is not supported. |
| `DEADLINE_EXCEEDED` | `DEADLINE_EXCEEDED` | yes | The caller's deadline elapsed before the request completed. |
| `FEATURE_NOT_IN_CURRENT_PHASE` | `UNIMPLEMENTED` | no | The requested field or method is defined in the contract but not yet implemented. |
| `INSUFFICIENT_SCOPE` | `PERMISSION_DENIED` | no | The authenticated principal lacks the scope this method requires. |
| `INTERNAL` | `INTERNAL` | no | An internal error occurred. |
| `INVALID_ARGUMENT` | `INVALID_ARGUMENT` | no | The request is malformed or its fields contradict each other. |
| `INVALID_TOKEN` | `UNAUTHENTICATED` | no | The bearer token could not be verified. |
| `KEY_EMPTY` | `INVALID_ARGUMENT` | no | The key is empty, which is only valid as a range boundary. |
| `KEY_TOO_LARGE` | `INVALID_ARGUMENT` | no | The key exceeds the configured maximum key length. |
| `MISSING_CREDENTIAL` | `UNAUTHENTICATED` | no | The request carried no usable bearer token. |
| `NOT_LEADER` | `FAILED_PRECONDITION` | yes | This node is not the leader for the requested keys. |
| `NO_LEADER` | `UNAVAILABLE` | yes | No leader is currently known for the requested keys. |
| `QUOTA_EXCEEDED` | `RESOURCE_EXHAUSTED` | yes | The database size quota is exhausted; writes are refused until space is reclaimed. |
| `RATE_LIMITED` | `RESOURCE_EXHAUSTED` | yes | The principal exceeded its request rate limit. |
| `REVISION_COMPACTED` | `OUT_OF_RANGE` | no | The requested revision is below the compaction point and is no longer available. |
| `SHUTTING_DOWN` | `UNAVAILABLE` | yes | The node is shutting down and is no longer accepting requests. |
| `STALE_READ_BOUND_UNMET` | `FAILED_PRECONDITION` | yes | This replica has not yet applied the minimum revision the read requires. |
| `UNKNOWN_KEY_ID` | `UNAUTHENTICATED` | no | The token's signing key is not known to this node. |
| `VALUE_TOO_LARGE` | `INVALID_ARGUMENT` | no | The value exceeds the configured maximum value size. |
| `WATCH_BUFFER_OVERFLOW` | `RESOURCE_EXHAUSTED` | yes | The watch was canceled because its consumer fell too far behind. |
<!-- catalog:end -->

The catalog is checked against the server's reason registry by an automated test, so it cannot drift from the code.
