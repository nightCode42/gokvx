package kverr_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// TestKindZeroValueIsInternal checks that a kind nobody set reports a server
// fault, so an unclassified error can never be mistaken for a client error.
func TestKindZeroValueIsInternal(t *testing.T) {
	t.Parallel()

	var zero kverr.Kind

	assert.Equal(t, kverr.KindInternal, zero)
	assert.Equal(t, "INTERNAL", zero.String())
	assert.False(t, zero.IsClientFault())
}

// TestKindString checks the name of every kind and the fallback for a value
// outside the enumeration.
func TestKindString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind kverr.Kind
		want string
	}{
		{kverr.KindInternal, "INTERNAL"},
		{kverr.KindInvalidArgument, "INVALID_ARGUMENT"},
		{kverr.KindNotFound, "NOT_FOUND"},
		{kverr.KindAlreadyExists, "ALREADY_EXISTS"},
		{kverr.KindPermissionDenied, "PERMISSION_DENIED"},
		{kverr.KindUnauthenticated, "UNAUTHENTICATED"},
		{kverr.KindResourceExhausted, "RESOURCE_EXHAUSTED"},
		{kverr.KindFailedPrecondition, "FAILED_PRECONDITION"},
		{kverr.KindAborted, "ABORTED"},
		{kverr.KindOutOfRange, "OUT_OF_RANGE"},
		{kverr.KindUnimplemented, "UNIMPLEMENTED"},
		{kverr.KindUnavailable, "UNAVAILABLE"},
		{kverr.KindDeadlineExceeded, "DEADLINE_EXCEEDED"},
		{kverr.KindCanceled, "CANCELED"},
		{kverr.Kind(200), "Kind(200)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.kind.String())
		})
	}
}

// TestKindIsClientFault checks which kinds blame the caller, which decides the
// log level at the transport edges.
// Verifies: KV-OBS-023.
func TestKindIsClientFault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind kverr.Kind
		want bool
	}{
		{kverr.KindInvalidArgument, true},
		{kverr.KindNotFound, true},
		{kverr.KindAlreadyExists, true},
		{kverr.KindPermissionDenied, true},
		{kverr.KindUnauthenticated, true},
		{kverr.KindOutOfRange, true},
		{kverr.KindUnimplemented, true},
		{kverr.KindCanceled, true},
		{kverr.KindInternal, false},
		{kverr.KindResourceExhausted, false},
		{kverr.KindFailedPrecondition, false},
		{kverr.KindAborted, false},
		{kverr.KindUnavailable, false},
		{kverr.KindDeadlineExceeded, false},
		{kverr.Kind(200), false},
	}

	for _, tt := range tests {
		t.Run(tt.kind.String(), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.kind.IsClientFault())
		})
	}
}
