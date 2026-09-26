package kverr_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// TestErrorString checks the text used in logs and traces: the reason, the
// message, then the cause chain when there is one.
func TestErrorString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *kverr.Error
		want string
	}{
		{
			name: "without cause",
			err:  kverr.New(kverr.ReasonKeyTooLarge, "key exceeds the maximum length"),
			want: "KEY_TOO_LARGE: key exceeds the maximum length",
		},
		{
			name: "with cause",
			err:  kverr.Wrap(kverr.ReasonInternal, errors.New("disk full"), "command log sync failed"),
			want: "INTERNAL: command log sync failed: disk full",
		},
		{
			name: "default message",
			err:  kverr.New(kverr.ReasonCanceled, ""),
			want: "CANCELED: The caller canceled the request.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}

// TestErrorKindAndRetryableComeFromRegistry_KV_API_091 checks that an error's
// kind and retryability are derived from its reason, never set separately.
// Verifies: KV-API-090, KV-API-091.
func TestErrorKindAndRetryableComeFromRegistry_KV_API_091(t *testing.T) {
	t.Parallel()

	for _, r := range kverr.AllReasons() {
		t.Run(string(r), func(t *testing.T) {
			t.Parallel()

			err := kverr.New(r, "message")

			assert.Equal(t, r, err.Reason())
			assert.Equal(t, kverr.KindOf(r), err.Kind())
			assert.Equal(t, kverr.Retryable(r), err.Retryable())
		})
	}
}

// TestUnwrapReturnsCause checks that the cause stays reachable for errors.Is
// and errors.As, and that an error without a cause unwraps to nil.
func TestUnwrapReturnsCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("connection reset")

	assert.Same(t, cause, kverr.Wrap(kverr.ReasonInternal, cause, "").Unwrap())
	assert.NoError(t, kverr.New(kverr.ReasonInternal, "").Unwrap())
}

// TestIsComparesByReason checks that two errors match under errors.Is when
// their reasons match, regardless of message, cause, or metadata.
func TestIsComparesByReason(t *testing.T) {
	t.Parallel()

	a := kverr.New(kverr.ReasonNotLeader, "first").WithMetadata("leader", "gokvx-1:2379")
	b := kverr.Wrap(kverr.ReasonNotLeader, errors.New("cause"), "second")
	other := kverr.New(kverr.ReasonNoLeader, "first")

	assert.ErrorIs(t, a, b)
	assert.ErrorIs(t, b, a)
	assert.NotErrorIs(t, a, other)
	assert.NotErrorIs(t, a, errors.New("NOT_LEADER: first"), "a plain error must never match")
}

// TestAccessorsReturnCopies checks that mutating what an accessor returns
// cannot change the error, which is what makes an Error safe to share.
func TestAccessorsReturnCopies(t *testing.T) {
	t.Parallel()

	err := kverr.New(kverr.ReasonKeyTooLarge, "").
		WithMetadata("limitBytes", "1024").
		WithViolations(kverr.FieldViolation{Field: "put.key", Description: "too long"})

	metadata := err.Metadata()
	metadata["limitBytes"] = "0"
	metadata["injected"] = "yes"

	violations := err.Violations()
	violations[0].Field = "changed"

	assert.Equal(t, map[string]string{"limitBytes": "1024"}, err.Metadata())
	assert.Equal(t, "put.key", err.Violations()[0].Field)
}

// TestLogValue checks the structured form of an error in logs, with and
// without a cause, and for a typed nil.
func TestLogValue(t *testing.T) {
	t.Parallel()

	withCause := kverr.Wrap(kverr.ReasonNoLeader, errors.New("dial timeout"), "no leader in time")
	withoutCause := kverr.New(kverr.ReasonKeyEmpty, "")
	var typedNil *kverr.Error

	assert.Equal(t,
		"[reason=NO_LEADER kind=UNAVAILABLE message=no leader in time cause=dial timeout]",
		withCause.LogValue().String())
	assert.Equal(t,
		"[reason=KEY_EMPTY kind=INVALID_ARGUMENT message=The key is empty, which is only valid as a range boundary.]",
		withoutCause.LogValue().String())
	assert.Equal(t, "<nil>", typedNil.LogValue().String())
}

// TestAccessorsOfPlainErrorAreEmpty checks the zero state of the optional
// parts of an error.
func TestAccessorsOfPlainErrorAreEmpty(t *testing.T) {
	t.Parallel()

	err := kverr.New(kverr.ReasonInternal, "")

	assert.Nil(t, err.Metadata())
	assert.Nil(t, err.Violations())
	assert.Equal(t, time.Duration(0), err.RetryAfter())
	require.NoError(t, err.Unwrap())
}
