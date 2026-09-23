package kverr_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// TestReasonOf checks how an arbitrary error is classified, including errors
// that never passed through this package.
func TestReasonOf(t *testing.T) {
	t.Parallel()

	var typedNil *kverr.Error

	tests := []struct {
		name string
		err  error
		want kverr.Reason
	}{
		{"nil error", nil, ""},
		{"kverr error", kverr.New(kverr.ReasonKeyEmpty, ""), kverr.ReasonKeyEmpty},
		{"wrapped once", fmt.Errorf("mvcc.Put: %w", kverr.New(kverr.ReasonQuotaExceeded, "")), kverr.ReasonQuotaExceeded},
		{
			"wrapped twice",
			fmt.Errorf("server.Put: %w", fmt.Errorf("mvcc.Put: %w", kverr.New(kverr.ReasonNotLeader, ""))),
			kverr.ReasonNotLeader,
		},
		{"plain error", errors.New("boom"), kverr.ReasonInternal},
		{"context canceled", context.Canceled, kverr.ReasonCanceled},
		{"wrapped deadline", fmt.Errorf("wait: %w", context.DeadlineExceeded), kverr.ReasonDeadlineExceeded},
		{"typed nil in chain", fmt.Errorf("x: %w", typedNil), kverr.ReasonInternal},
		{
			"kverr wins over its cause",
			kverr.Wrap(kverr.ReasonShuttingDown, context.Canceled, ""),
			kverr.ReasonShuttingDown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, kverr.ReasonOf(tt.err))
		})
	}
}

// TestErrorsIsThroughWrapChain checks that a sentinel matches an error created
// deep in the stack and wrapped on the way up, the way errors travel through
// gokvx's layers.
func TestErrorsIsThroughWrapChain(t *testing.T) {
	t.Parallel()

	origin := kverr.New(kverr.ReasonNotLeader, "write must go to the leader").
		WithMetadata("leader", "gokvx-1:2379")
	err := fmt.Errorf("server.Put: %w", fmt.Errorf("consensus.Propose: %w", origin))

	assert.ErrorIs(t, err, kverr.ErrNotLeader)
	assert.NotErrorIs(t, err, kverr.ErrNoLeader)
	assert.NotErrorIs(t, err, kverr.ErrShuttingDown)
}

// TestErrorsAsThroughWrapChain checks that the original error, with its
// metadata, can be extracted from a wrapped chain.
func TestErrorsAsThroughWrapChain(t *testing.T) {
	t.Parallel()

	origin := kverr.New(kverr.ReasonKeyTooLarge, "").WithMetadata("limitBytes", "1024")
	err := fmt.Errorf("server.Put: %w", fmt.Errorf("mvcc.Put: %w", origin))

	var target *kverr.Error
	require.ErrorAs(t, err, &target)

	assert.Same(t, origin, target)
	assert.Equal(t, "1024", target.Metadata()["limitBytes"])
	assert.Equal(t, kverr.KindInvalidArgument, target.Kind())
}

// TestErrorsIsReachesForeignCause checks that a foreign cause stays
// reachable beneath a kverr error, so callers can still test for it.
func TestErrorsIsReachesForeignCause(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("server.Get: %w", kverr.Wrap(kverr.ReasonInternal, context.DeadlineExceeded, ""))

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, kverr.ReasonInternal, kverr.ReasonOf(err), "the first kverr in the chain decides the reason")
}

// TestSentinels checks that every sentinel is a distinct, registered reason
// and carries its client-safe default message.
func TestSentinels(t *testing.T) {
	t.Parallel()

	sentinels := []*kverr.Error{
		kverr.ErrRevisionCompacted,
		kverr.ErrNotLeader,
		kverr.ErrNoLeader,
		kverr.ErrQuotaExceeded,
		kverr.ErrShuttingDown,
		kverr.ErrFeatureNotInCurrentPhase,
	}

	seen := make(map[kverr.Reason]bool, len(sentinels))
	for _, s := range sentinels {
		assert.True(t, kverr.Registered(s.Reason()), "sentinel %q is not registered", s.Reason())
		assert.False(t, seen[s.Reason()], "two sentinels share reason %q", s.Reason())
		assert.Equal(t, kverr.Description(s.Reason()), s.Message())
		seen[s.Reason()] = true
	}
}

// TestTypedNilIsNotNil documents why functions in gokvx return error and never
// *kverr.Error: a nil *kverr.Error stored in an error interface is not nil, so
// a caller's "if err != nil" is silently true.
func TestTypedNilIsNotNil(t *testing.T) {
	t.Parallel()

	// returnsConcrete follows the forbidden pattern and reports success.
	returnsConcrete := func() *kverr.Error { return nil }
	// returnsInterface follows the rule and reports success.
	returnsInterface := func() error { return nil }

	var viaConcrete error = returnsConcrete()

	// testify's Nil and NotNil look through the interface and treat a typed
	// nil as nil, which would hide the trap; compare with nil directly.
	assert.False(t, isNilError(viaConcrete), "a typed nil stored in error is not nil")
	assert.True(t, isNilError(returnsInterface()), "returning the error interface keeps nil nil")
}

// TestTypedNilInChainDoesNotPanic checks that the errors package can walk a
// chain containing a typed-nil *kverr.Error, which calls its methods on a nil
// receiver.
func TestTypedNilInChainDoesNotPanic(t *testing.T) {
	t.Parallel()

	var typedNil *kverr.Error
	err := fmt.Errorf("layer: %w", typedNil)

	assert.NotPanics(t, func() {
		_ = errors.Is(err, kverr.ErrNotLeader)
		_ = errors.Is(err, context.Canceled)
		_ = err.Error()
	})
	assert.NotErrorIs(t, err, kverr.ErrNotLeader)
}

// isNilError reports whether err is the nil interface value, exactly as an
// "if err != nil" check in production code would see it.
func isNilError(err error) bool {
	return err == nil
}
