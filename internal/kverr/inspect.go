package kverr

import (
	"context"
	"errors"
)

// Sentinel errors for the reasons that code inside gokvx routinely branches
// on. They compare by reason, so errors.Is(err, ErrNotLeader) matches every
// error with reason NOT_LEADER, whatever its message or cause. They cannot be
// modified, because an Error has no exported fields.
var (
	// ErrRevisionCompacted matches reads and watches below the compaction
	// point.
	ErrRevisionCompacted = New(ReasonRevisionCompacted, "")
	// ErrNotLeader matches writes rejected by a follower.
	ErrNotLeader = New(ReasonNotLeader, "")
	// ErrNoLeader matches requests that found no leader in time.
	ErrNoLeader = New(ReasonNoLeader, "")
	// ErrQuotaExceeded matches writes refused by the database size quota.
	ErrQuotaExceeded = New(ReasonQuotaExceeded, "")
	// ErrShuttingDown matches requests refused while the node drains.
	ErrShuttingDown = New(ReasonShuttingDown, "")
	// ErrFeatureNotInCurrentPhase matches fields and methods that are defined
	// but not yet functional.
	ErrFeatureNotInCurrentPhase = New(ReasonFeatureNotInCurrentPhase, "")
)

// ReasonOf returns the reason of the first *[Error] in err's chain.
//
// A nil error has no reason and returns the empty Reason. A chain without an
// *Error is classified by what it contains: context.Canceled and
// context.DeadlineExceeded become [ReasonCanceled] and
// [ReasonDeadlineExceeded]; anything else is [ReasonInternal], because an
// error nobody classified is a server fault.
func ReasonOf(err error) Reason {
	if err == nil {
		return ""
	}

	// A typed-nil *Error in the chain is a bug in the code that returned it;
	// it carries no reason, so it falls through to the classification below.
	var e *Error
	if errors.As(err, &e) && e != nil {
		return e.reason
	}

	switch {
	case errors.Is(err, context.Canceled):
		return ReasonCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return ReasonDeadlineExceeded
	default:
		return ReasonInternal
	}
}
