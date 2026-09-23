package kverr

import "slices"

// Reason is the stable, machine-readable identifier of a failure.
//
// A reason is sent to clients in the google.rpc.ErrorInfo detail of a gRPC
// status (KV-API-090) and is documented in docs/errors.md (KV-API-092). It is
// part of the public contract: a reason is never renamed and never given a new
// meaning. A reason that falls out of use is retired, not reused.
type Reason string

// The reasons gokvx can report. Each is registered in [reasons] with the kind
// that classifies it and whether retrying may help.
const (
	// ReasonKeyEmpty is an empty key outside a range boundary (KV-DAT-012).
	ReasonKeyEmpty Reason = "KEY_EMPTY"
	// ReasonKeyTooLarge is a key above limits.max_key_bytes (KV-DAT-011).
	ReasonKeyTooLarge Reason = "KEY_TOO_LARGE"
	// ReasonValueTooLarge is a value above limits.max_value_bytes
	// (KV-DAT-011).
	ReasonValueTooLarge Reason = "VALUE_TOO_LARGE"
	// ReasonInvalidArgument is a malformed or contradictory request that no
	// more specific reason describes.
	ReasonInvalidArgument Reason = "INVALID_ARGUMENT"

	// ReasonRevisionCompacted is a read, scan, or watch below the compaction
	// point (KV-DAT-005).
	ReasonRevisionCompacted Reason = "REVISION_COMPACTED"

	// ReasonMissingCredential is absent or malformed authorization metadata
	// (KV-SEC-010).
	ReasonMissingCredential Reason = "MISSING_CREDENTIAL"
	// ReasonInvalidToken is a token whose signature or claims did not
	// validate (KV-SEC-014, KV-SEC-015).
	ReasonInvalidToken Reason = "INVALID_TOKEN"
	// ReasonUnknownKeyID is a token whose kid did not resolve in the JWKS
	// cache after the permitted refresh (KV-SEC-016).
	ReasonUnknownKeyID Reason = "UNKNOWN_KEY_ID"
	// ReasonInsufficientScope is a principal whose scopes do not cover the
	// method (KV-SEC-020).
	ReasonInsufficientScope Reason = "INSUFFICIENT_SCOPE"

	// ReasonRateLimited is a principal exceeding its rate limit
	// (KV-SEC-032).
	ReasonRateLimited Reason = "RATE_LIMITED"
	// ReasonQuotaExceeded is the database size quota being reached; writes
	// are refused while reads continue (KV-STO-010).
	ReasonQuotaExceeded Reason = "QUOTA_EXCEEDED"
	// ReasonWatchBufferOverflow is a watch canceled because its consumer
	// could not keep up (KV-API-045).
	ReasonWatchBufferOverflow Reason = "WATCH_BUFFER_OVERFLOW"

	// ReasonNotLeader is a write received by a follower that is configured to
	// reject rather than forward (KV-CON-008).
	ReasonNotLeader Reason = "NOT_LEADER"
	// ReasonNoLeader is no leader being known within the configured wait
	// (KV-CON-009).
	ReasonNoLeader Reason = "NO_LEADER"
	// ReasonStaleReadBoundUnmet is a STALE read whose min_revision exceeds
	// the local applied revision (KV-API-011).
	ReasonStaleReadBoundUnmet Reason = "STALE_READ_BOUND_UNMET"
	// ReasonCrossShardTxnUnsupported is a transaction spanning shard groups
	// (KV-API-051).
	ReasonCrossShardTxnUnsupported Reason = "CROSS_SHARD_TXN_UNSUPPORTED"

	// ReasonFeatureNotInCurrentPhase is a field or RPC that is defined in the
	// contract but not yet functional (KV-API-000).
	ReasonFeatureNotInCurrentPhase Reason = "FEATURE_NOT_IN_CURRENT_PHASE"

	// ReasonShuttingDown is a node draining after SIGTERM (KV-CFG-010).
	ReasonShuttingDown Reason = "SHUTTING_DOWN"
	// ReasonDeadlineExceeded is the caller's deadline elapsing before the
	// request completed.
	ReasonDeadlineExceeded Reason = "DEADLINE_EXCEEDED"
	// ReasonCanceled is the caller canceling the request.
	ReasonCanceled Reason = "CANCELED"

	// ReasonInternal is an invariant violation or an unexpected failure. It
	// is the fallback for any error that was never classified.
	ReasonInternal Reason = "INTERNAL"
)

// reasonInfo is what the registry records about one reason.
type reasonInfo struct {
	// kind classifies the reason and determines its status code at the edges.
	kind Kind
	// retryable reports whether repeating the same request unchanged, after a
	// delay, may succeed. It drives the google.rpc.RetryInfo detail
	// (KV-API-091). Failures that need a different request or a new
	// credential are not retryable, even when a client may sensibly try again
	// after changing something.
	retryable bool
	// description is one sentence for docs/errors.md (KV-API-092) and for the
	// default client-facing message.
	description string
}

// reasons is the single registry of every reason gokvx can report. It is the
// source of truth for the kind and retryability of each one, so a reason can
// never be paired with the wrong status code.
var reasons = map[Reason]reasonInfo{
	ReasonKeyEmpty: {
		kind:        KindInvalidArgument,
		description: "The key is empty, which is only valid as a range boundary.",
	},
	ReasonKeyTooLarge: {
		kind:        KindInvalidArgument,
		description: "The key exceeds the configured maximum key length.",
	},
	ReasonValueTooLarge: {
		kind:        KindInvalidArgument,
		description: "The value exceeds the configured maximum value size.",
	},
	ReasonInvalidArgument: {
		kind:        KindInvalidArgument,
		description: "The request is malformed or its fields contradict each other.",
	},
	ReasonRevisionCompacted: {
		kind:        KindOutOfRange,
		description: "The requested revision is below the compaction point and is no longer available.",
	},
	ReasonMissingCredential: {
		kind:        KindUnauthenticated,
		description: "The request carried no usable bearer token.",
	},
	ReasonInvalidToken: {
		kind:        KindUnauthenticated,
		description: "The bearer token could not be verified.",
	},
	ReasonUnknownKeyID: {
		kind:        KindUnauthenticated,
		description: "The token's signing key is not known to this node.",
	},
	ReasonInsufficientScope: {
		kind:        KindPermissionDenied,
		description: "The authenticated principal lacks the scope this method requires.",
	},
	ReasonRateLimited: {
		kind:        KindResourceExhausted,
		retryable:   true,
		description: "The principal exceeded its request rate limit.",
	},
	ReasonQuotaExceeded: {
		kind:        KindResourceExhausted,
		retryable:   true,
		description: "The database size quota is exhausted; writes are refused until space is reclaimed.",
	},
	ReasonWatchBufferOverflow: {
		kind:        KindResourceExhausted,
		retryable:   true,
		description: "The watch was canceled because its consumer fell too far behind.",
	},
	ReasonNotLeader: {
		kind:        KindFailedPrecondition,
		retryable:   true,
		description: "This node is not the leader for the requested keys.",
	},
	ReasonNoLeader: {
		kind:        KindUnavailable,
		retryable:   true,
		description: "No leader is currently known for the requested keys.",
	},
	ReasonStaleReadBoundUnmet: {
		kind:        KindFailedPrecondition,
		retryable:   true,
		description: "This replica has not yet applied the minimum revision the read requires.",
	},
	ReasonCrossShardTxnUnsupported: {
		kind:        KindFailedPrecondition,
		description: "The transaction spans more than one shard group, which is not supported.",
	},
	ReasonFeatureNotInCurrentPhase: {
		kind:        KindUnimplemented,
		description: "The requested field or method is defined in the contract but not yet implemented.",
	},
	ReasonShuttingDown: {
		kind:        KindUnavailable,
		retryable:   true,
		description: "The node is shutting down and is no longer accepting requests.",
	},
	ReasonDeadlineExceeded: {
		kind:        KindDeadlineExceeded,
		retryable:   true,
		description: "The caller's deadline elapsed before the request completed.",
	},
	ReasonCanceled: {
		kind:        KindCanceled,
		description: "The caller canceled the request.",
	},
	ReasonInternal: {
		kind:        KindInternal,
		description: "An internal error occurred.",
	},
}

// KindOf returns the kind registered for r, or [KindInternal] when r is not
// registered, so an unregistered reason is treated as a server fault.
func KindOf(r Reason) Kind {
	info, ok := reasons[r]
	if !ok {
		return KindInternal
	}
	return info.kind
}

// Retryable reports whether repeating the same request unchanged may succeed
// after a delay. An unregistered reason is not retryable.
func Retryable(r Reason) bool {
	return reasons[r].retryable
}

// Description returns the one-sentence description registered for r, or an
// empty string when r is not registered.
func Description(r Reason) string {
	return reasons[r].description
}

// Registered reports whether r is a known reason.
func Registered(r Reason) bool {
	_, ok := reasons[r]
	return ok
}

// AllReasons returns every registered reason in lexical order. The order is
// fixed so tests are deterministic and can compare it with the published
// catalog in docs/errors.md, which lists reasons in the same order.
func AllReasons() []Reason {
	all := make([]Reason, 0, len(reasons))
	for r := range reasons {
		all = append(all, r)
	}
	slices.Sort(all)
	return all
}
