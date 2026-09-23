package kverr

import (
	"fmt"
	"maps"
	"slices"
	"time"
)

// New returns an error with the given reason and client-safe message.
//
// An empty message is replaced by the reason's registered description, so
// every error carries a message a client can read. A reason that is not
// registered becomes [ReasonInternal], so only reasons from the documented
// catalog ever reach clients (KV-API-090, KV-API-092).
func New(reason Reason, message string) *Error {
	if !Registered(reason) {
		reason = ReasonInternal
	}
	if message == "" {
		message = Description(reason)
	}
	return &Error{reason: reason, message: message}
}

// Newf is [New] with a message formatted by fmt.Sprintf. The arguments must
// be client-safe: never keys, values, or token contents.
func Newf(reason Reason, format string, args ...any) *Error {
	return New(reason, fmt.Sprintf(format, args...))
}

// Wrap returns an error with the given reason and client-safe message whose
// cause is a foreign error — one from the standard library, Pebble, or the
// network — entering gokvx code. Wrap it once, where it enters; above that
// point, add context with fmt.Errorf and %w instead.
func Wrap(reason Reason, cause error, message string) *Error {
	e := New(reason, message)
	e.cause = cause
	return e
}

// WithMetadata returns a copy of the error with one metadata entry added or
// replaced. Metadata is sent to clients in google.rpc.ErrorInfo, so it must
// be non-sensitive. Keys follow the ErrorInfo convention: lower camel case
// with the unit in the key, for example "limitBytes" = "1024".
func (e *Error) WithMetadata(key, value string) *Error {
	c := e.clone()
	if c.metadata == nil {
		c.metadata = make(map[string]string, 1)
	}
	c.metadata[key] = value
	return c
}

// WithViolations returns a copy of the error with field violations appended.
// Violations are meant for errors of kind [KindInvalidArgument].
func (e *Error) WithViolations(violations ...FieldViolation) *Error {
	c := e.clone()
	c.violations = append(c.violations, violations...)
	return c
}

// WithRetryAfter returns a copy of the error with a suggested backoff. A
// negative duration is treated as zero, meaning the client's own default. The
// hint is only sent to clients when the error is retryable (KV-API-091).
func (e *Error) WithRetryAfter(d time.Duration) *Error {
	c := e.clone()
	c.retryAfter = max(d, 0)
	return c
}

// clone returns a deep copy of the error, so a With* method can change the
// copy without the original ever observing it.
func (e *Error) clone() *Error {
	c := *e
	c.metadata = maps.Clone(e.metadata)
	c.violations = slices.Clone(e.violations)
	return &c
}
