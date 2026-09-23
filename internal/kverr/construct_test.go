package kverr_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// TestNewKeepsReasonAndMessage checks the basic constructor.
func TestNewKeepsReasonAndMessage(t *testing.T) {
	t.Parallel()

	err := kverr.New(kverr.ReasonKeyTooLarge, "key exceeds the maximum length")

	assert.Equal(t, kverr.ReasonKeyTooLarge, err.Reason())
	assert.Equal(t, "key exceeds the maximum length", err.Message())
	assert.NoError(t, err.Unwrap())
}

// TestNewDefaultsEmptyMessageToDescription checks that every error carries a
// client-readable message, even when the caller gave none.
func TestNewDefaultsEmptyMessageToDescription(t *testing.T) {
	t.Parallel()

	for _, r := range kverr.AllReasons() {
		t.Run(string(r), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, kverr.Description(r), kverr.New(r, "").Message())
		})
	}
}

// TestNewReplacesUnregisteredReason_KV_API_090 checks that a reason outside
// the documented catalog can never reach a client.
// Verifies: KV-API-090, KV-API-092.
func TestNewReplacesUnregisteredReason_KV_API_090(t *testing.T) {
	t.Parallel()

	err := kverr.New(kverr.Reason("NOT_IN_THE_CATALOG"), "something failed")

	assert.Equal(t, kverr.ReasonInternal, err.Reason())
	assert.Equal(t, kverr.KindInternal, err.Kind())
	assert.Equal(t, "something failed", err.Message(), "the caller's message is kept")
}

// TestNewfFormatsMessage checks the formatting constructor.
func TestNewfFormatsMessage(t *testing.T) {
	t.Parallel()

	err := kverr.Newf(kverr.ReasonKeyTooLarge, "key exceeds %d bytes", 1024)

	assert.Equal(t, kverr.ReasonKeyTooLarge, err.Reason())
	assert.Equal(t, "key exceeds 1024 bytes", err.Message())
}

// TestWrapKeepsCause checks that a wrapped foreign error stays reachable
// through the standard library's inspection functions.
func TestWrapKeepsCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("fsync: input/output error")

	err := kverr.Wrap(kverr.ReasonInternal, cause, "command log sync failed")

	assert.Equal(t, kverr.ReasonInternal, err.Reason())
	assert.Equal(t, "command log sync failed", err.Message())
	assert.ErrorIs(t, err, cause)
}

// TestWithMetadataDoesNotMutateReceiver checks that WithMetadata returns a
// changed copy and leaves the original untouched.
func TestWithMetadataDoesNotMutateReceiver(t *testing.T) {
	t.Parallel()

	original := kverr.New(kverr.ReasonKeyTooLarge, "").WithMetadata("limitBytes", "1024")

	changed := original.WithMetadata("limitBytes", "2048").WithMetadata("actualBytes", "4096")

	assert.Equal(t, map[string]string{"limitBytes": "1024"}, original.Metadata())
	assert.Equal(t, map[string]string{"limitBytes": "2048", "actualBytes": "4096"}, changed.Metadata())
}

// TestWithViolationsDoesNotMutateReceiver checks that appending violations to
// a copy never shows up in the original, even when both share history.
func TestWithViolationsDoesNotMutateReceiver(t *testing.T) {
	t.Parallel()

	first := kverr.FieldViolation{Field: "put.key", Description: "is empty"}
	second := kverr.FieldViolation{Field: "put.value", Description: "is too large"}
	third := kverr.FieldViolation{Field: "compare.target", Description: "is unspecified"}

	base := kverr.New(kverr.ReasonInvalidArgument, "").WithViolations(first)
	left := base.WithViolations(second)
	right := base.WithViolations(third)

	assert.Equal(t, []kverr.FieldViolation{first}, base.Violations())
	assert.Equal(t, []kverr.FieldViolation{first, second}, left.Violations())
	assert.Equal(t, []kverr.FieldViolation{first, third}, right.Violations())
}

// TestWithRetryAfter_KV_API_091 checks the backoff hint, including that a
// negative duration is treated as "use the client's default".
// Verifies: KV-API-091.
func TestWithRetryAfter_KV_API_091(t *testing.T) {
	t.Parallel()

	original := kverr.New(kverr.ReasonRateLimited, "")

	assert.Equal(t, 2*time.Second, original.WithRetryAfter(2*time.Second).RetryAfter())
	assert.Equal(t, time.Duration(0), original.WithRetryAfter(-time.Second).RetryAfter())
	assert.Equal(t, time.Duration(0), original.RetryAfter(), "the original is unchanged")
}

// TestWithMethodsKeepIdentity checks that the copy methods change only what
// they are asked to change.
func TestWithMethodsKeepIdentity(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause")
	original := kverr.Wrap(kverr.ReasonNoLeader, cause, "no leader in time")

	changed := original.
		WithMetadata("waitedMs", "3000").
		WithRetryAfter(time.Second).
		WithViolations(kverr.FieldViolation{Field: "f", Description: "d"})

	assert.Equal(t, original.Reason(), changed.Reason())
	assert.Equal(t, original.Message(), changed.Message())
	assert.ErrorIs(t, changed, cause)
}
