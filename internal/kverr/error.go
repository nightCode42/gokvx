package kverr

import (
	"maps"
	"slices"
	"time"
)

// FieldViolation describes one invalid field in a request. At the gRPC edge it
// becomes a google.rpc.BadRequest.FieldViolation.
type FieldViolation struct {
	// Field is the path of the invalid field in the request, for example
	// "put.key" or "compare.target".
	Field string
	// Description explains the constraint the field violated. Like a
	// message, it is shown to clients and never contains keys or values.
	Description string
}

// Error is the single error type used across gokvx.
//
// Its fields are unexported, so an Error can only be built by [New], [Newf],
// or [Wrap], and it is never modified after creation: the With* methods
// return a changed copy, and the accessors return copies of the metadata and
// violations. An Error is therefore safe to share between goroutines and to
// keep in a package-level sentinel.
//
// An Error separates what a client may see from what only operators may see
// (KV-SEC-030). [Error.Message], [Error.Metadata], and [Error.Violations] are
// client-safe; the cause returned by [Error.Unwrap] and the full text of
// [Error.Error] are for logs and traces only.
type Error struct {
	// reason identifies the failure; always a registered reason.
	reason Reason
	// message is the client-safe explanation; never empty.
	message string
	// metadata is non-sensitive context for google.rpc.ErrorInfo.metadata.
	metadata map[string]string
	// violations lists invalid request fields for google.rpc.BadRequest.
	violations []FieldViolation
	// retryAfter is the suggested backoff for google.rpc.RetryInfo; zero
	// means the client's own default.
	retryAfter time.Duration
	// cause is the underlying error, if any; never sent to clients.
	cause error
}

// Reason returns the stable identifier of the failure.
func (e *Error) Reason() Reason {
	return e.reason
}

// Kind returns the class of the failure, derived from its reason.
func (e *Error) Kind() Kind {
	return KindOf(e.reason)
}

// Retryable reports whether repeating the same request unchanged may succeed
// after a delay (KV-API-091).
func (e *Error) Retryable() bool {
	return Retryable(e.reason)
}

// Message returns the client-safe explanation of the failure.
func (e *Error) Message() string {
	return e.message
}

// Metadata returns a copy of the error's non-sensitive context, or nil if it
// has none.
func (e *Error) Metadata() map[string]string {
	return maps.Clone(e.metadata)
}

// Violations returns a copy of the error's field violations, or nil if it has
// none.
func (e *Error) Violations() []FieldViolation {
	return slices.Clone(e.violations)
}

// RetryAfter returns the suggested backoff before retrying. Zero means the
// client should use its own default.
func (e *Error) RetryAfter() time.Duration {
	return e.retryAfter
}

// Error returns the reason, the message, and the cause chain, for example
// "INTERNAL: command log sync failed: fsync /data/0007.log: i/o error".
//
// The text includes the cause, which may hold internal detail, so it is meant
// for logs and traces. Clients receive [Error.Message] instead.
//
// Error, Unwrap, and Is tolerate a nil receiver, because the errors and fmt
// packages call them on every link of a chain; a typed-nil *Error returned by
// mistake must not turn error handling into a panic.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	text := string(e.reason) + ": " + e.message
	if e.cause != nil {
		text += ": " + e.cause.Error()
	}
	return text
}

// Unwrap returns the underlying cause, so errors.Is and errors.As can see
// through an Error to the failure that produced it.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is reports whether target is an *Error with the same reason. It lets
// errors.Is match a sentinel such as [ErrNotLeader] against any error with
// that reason, whatever its message or cause.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && e != nil && t != nil && e.reason == t.reason
}
