package kverr

import "strconv"

// Kind classifies an error by how a caller should react to it.
//
// Each kind maps to exactly one gRPC status code in gokvx and to one default
// HTTP status in microservice-1 (spec Appendix B). The mapping lives at those
// edges, not here.
//
// The zero value is [KindInternal], so an error that was never classified is
// reported as a server fault rather than mistaken for a client error.
type Kind uint8

const (
	// KindInternal is an invariant violation or an unexpected failure. It is
	// always accompanied by an ERROR log and never exposes detail to clients.
	KindInternal Kind = iota

	// KindInvalidArgument is a malformed or contradictory request, such as an
	// oversized value or an empty key. Retrying it unchanged cannot succeed.
	KindInvalidArgument

	// KindNotFound is a resource that must exist but does not, such as an
	// unknown lease. An absent key is not an error (KV-API-010).
	KindNotFound

	// KindAlreadyExists is a creation conflict on a uniquely named resource.
	KindAlreadyExists

	// KindPermissionDenied is an authenticated principal whose scopes or key
	// prefixes do not cover the request (KV-SEC-020).
	KindPermissionDenied

	// KindUnauthenticated is a missing, malformed, expired, or unverifiable
	// token. A caller may retry once with a fresh token (MS1-SEC-004).
	KindUnauthenticated

	// KindResourceExhausted is a limit reached: a rate limit, the database
	// quota, or a watch buffer. Retrying later, with backoff, may succeed.
	KindResourceExhausted

	// KindFailedPrecondition is a request that cannot be served in the node's
	// current state, such as a write on a follower or an unmet stale-read
	// bound. Retrying after a redirect or a wait may succeed.
	KindFailedPrecondition

	// KindAborted is a concurrency conflict, such as a slot migration handing
	// over ownership mid-request.
	KindAborted

	// KindOutOfRange is a read or scan below the compaction point
	// (KV-DAT-005). The data is gone; retrying cannot help.
	KindOutOfRange

	// KindUnimplemented is an RPC or field that is not functional in the
	// current delivery phase (KV-API-000).
	KindUnimplemented

	// KindUnavailable is a node that is shutting down, has no known leader, or
	// cannot be reached. Retrying, possibly on another node, may succeed.
	KindUnavailable

	// KindDeadlineExceeded is a caller's deadline elapsing before the request
	// completed.
	KindDeadlineExceeded

	// KindCanceled is a caller canceling the request before it completed.
	KindCanceled
)

// kindNames holds the name of each kind, indexed by its value. The names match
// the gRPC status codes the kinds map to, so logs and test failures read the
// same as the statuses clients receive.
var kindNames = [...]string{
	KindInternal:           "INTERNAL",
	KindInvalidArgument:    "INVALID_ARGUMENT",
	KindNotFound:           "NOT_FOUND",
	KindAlreadyExists:      "ALREADY_EXISTS",
	KindPermissionDenied:   "PERMISSION_DENIED",
	KindUnauthenticated:    "UNAUTHENTICATED",
	KindResourceExhausted:  "RESOURCE_EXHAUSTED",
	KindFailedPrecondition: "FAILED_PRECONDITION",
	KindAborted:            "ABORTED",
	KindOutOfRange:         "OUT_OF_RANGE",
	KindUnimplemented:      "UNIMPLEMENTED",
	KindUnavailable:        "UNAVAILABLE",
	KindDeadlineExceeded:   "DEADLINE_EXCEEDED",
	KindCanceled:           "CANCELED",
}

// String returns the kind's name, or Kind(<n>) for a value outside the
// enumeration, so an unknown kind is still printable in a log or a test
// failure.
func (k Kind) String() string {
	if int(k) >= len(kindNames) {
		return "Kind(" + strconv.Itoa(int(k)) + ")"
	}
	return kindNames[k]
}

// IsClientFault reports whether the kind blames the caller rather than the
// service. Edges use it to decide the log level: a client fault is logged at
// DEBUG, so log volume cannot be driven by a misbehaving client (KV-OBS-023).
func (k Kind) IsClientFault() bool {
	switch k {
	case KindInvalidArgument,
		KindNotFound,
		KindAlreadyExists,
		KindPermissionDenied,
		KindUnauthenticated,
		KindOutOfRange,
		KindUnimplemented,
		KindCanceled:
		return true
	case KindInternal,
		KindResourceExhausted,
		KindFailedPrecondition,
		KindAborted,
		KindUnavailable,
		KindDeadlineExceeded:
		return false
	default:
		return false
	}
}
