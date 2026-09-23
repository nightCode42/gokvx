// Package kverr defines the single error type used across gokvx and
// microservice-1.
//
// An error is created where a failure is detected, travels through every layer
// unchanged, and is translated exactly once at each edge: into a gRPC status in
// gokvx and into an HTTP response in microservice-1. Only those edges perform
// that translation (spec §6.2, rule A-5), which is why this package depends on
// neither gRPC nor net/http and can therefore be used by the storage and
// replication layers.
//
// # Reasons and kinds
//
// Every error carries a [Reason]: a stable, machine-readable identifier such as
// KEY_TOO_LARGE. Reasons are part of the public contract. They are sent to
// clients in the google.rpc.ErrorInfo detail of a gRPC status (KV-API-090),
// documented in docs/errors.md (KV-API-092), and never renamed or repurposed.
//
// Each reason is registered once, together with the [Kind] that classifies it
// and whether it is retryable (KV-API-091). The kind is derived from the
// reason, so a reason can never be paired with the wrong status code.
//
// # Using this package
//
// Functions return the error interface, never a concrete *[Error]: a nil
// *Error stored in an error interface is not nil, and the resulting "err !=
// nil" is silently true. Construct errors with [New], [Newf], or [Wrap], add
// context while crossing a boundary with fmt.Errorf and %w, and inspect them
// with [ReasonOf], errors.Is, or errors.As.
//
// The full model, including the translation tables for both edges, is described
// in docs/engineering/error-handling.md.
package kverr
