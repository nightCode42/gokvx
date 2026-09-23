package kverr_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// declaredReasons lists every reason constant the package exports. It is
// maintained by hand so that adding a reason without registering it, or
// registering one without declaring it, fails the test below rather than
// reaching clients as an unclassified error.
var declaredReasons = []kverr.Reason{
	kverr.ReasonKeyEmpty,
	kverr.ReasonKeyTooLarge,
	kverr.ReasonValueTooLarge,
	kverr.ReasonInvalidArgument,
	kverr.ReasonRevisionCompacted,
	kverr.ReasonMissingCredential,
	kverr.ReasonInvalidToken,
	kverr.ReasonUnknownKeyID,
	kverr.ReasonInsufficientScope,
	kverr.ReasonRateLimited,
	kverr.ReasonQuotaExceeded,
	kverr.ReasonWatchBufferOverflow,
	kverr.ReasonNotLeader,
	kverr.ReasonNoLeader,
	kverr.ReasonStaleReadBoundUnmet,
	kverr.ReasonCrossShardTxnUnsupported,
	kverr.ReasonFeatureNotInCurrentPhase,
	kverr.ReasonShuttingDown,
	kverr.ReasonDeadlineExceeded,
	kverr.ReasonCanceled,
	kverr.ReasonInternal,
}

// TestRegistryCoversEveryDeclaredReason_KV_API_092 checks that the declared
// reasons and the registry describe exactly the same set.
// Verifies: KV-API-092.
func TestRegistryCoversEveryDeclaredReason_KV_API_092(t *testing.T) {
	t.Parallel()

	want := slices.Clone(declaredReasons)
	slices.Sort(want)

	assert.Equal(t, want, kverr.AllReasons())
}

// TestEveryReasonHasKindAndDescription_KV_API_092 checks that no registered
// reason is missing the metadata the error catalog and the transport edges
// depend on.
// Verifies: KV-API-090, KV-API-092.
func TestEveryReasonHasKindAndDescription_KV_API_092(t *testing.T) {
	t.Parallel()

	for _, r := range kverr.AllReasons() {
		t.Run(string(r), func(t *testing.T) {
			t.Parallel()

			require.True(t, kverr.Registered(r), "reason is not registered")

			desc := kverr.Description(r)
			require.NotEmpty(t, desc, "reason has no description")
			assert.True(t, strings.HasSuffix(desc, "."), "description is not a sentence: %q", desc)

			kind := kverr.KindOf(r)
			assert.NotContains(t, kind.String(), "Kind(", "reason maps to an unnamed kind")
		})
	}
}

// TestReasonNamesAreUpperSnakeCase_KV_API_092 checks the wire format of the
// reason strings, which clients match on.
// Verifies: KV-API-092.
func TestReasonNamesAreUpperSnakeCase_KV_API_092(t *testing.T) {
	t.Parallel()

	for _, r := range kverr.AllReasons() {
		name := string(r)
		assert.Equal(t, strings.ToUpper(name), name, "reason is not upper case")
		assert.NotContains(t, name, " ", "reason contains a space")
		assert.NotContains(t, name, "-", "reason contains a hyphen; use underscores")
	}
}

// TestRetryableReasonsMatchTheirKind_KV_API_091 checks that only reasons whose
// kind can succeed on a later attempt are marked retryable, so RetryInfo is
// never attached to a failure that repeating cannot fix.
// Verifies: KV-API-091.
func TestRetryableReasonsMatchTheirKind_KV_API_091(t *testing.T) {
	t.Parallel()

	retryableKinds := []kverr.Kind{
		kverr.KindResourceExhausted,
		kverr.KindFailedPrecondition,
		kverr.KindAborted,
		kverr.KindUnavailable,
		kverr.KindDeadlineExceeded,
	}

	for _, r := range kverr.AllReasons() {
		if !kverr.Retryable(r) {
			continue
		}
		kind := kverr.KindOf(r)
		assert.Contains(t, retryableKinds, kind,
			"reason %q is retryable but its kind %s is not", r, kind)
	}
}

// TestUnregisteredReasonIsTreatedAsInternal checks the fallback for a reason
// that is not in the registry: it is a server fault, not a client error, and
// it is not retryable.
func TestUnregisteredReasonIsTreatedAsInternal(t *testing.T) {
	t.Parallel()

	const unknown = kverr.Reason("NOT_A_REGISTERED_REASON")

	assert.False(t, kverr.Registered(unknown))
	assert.Equal(t, kverr.KindInternal, kverr.KindOf(unknown))
	assert.False(t, kverr.Retryable(unknown))
	assert.Empty(t, kverr.Description(unknown))
}

// TestAllReasonsReturnsACopy checks that a caller cannot corrupt the registry
// order by modifying the returned slice.
func TestAllReasonsReturnsACopy(t *testing.T) {
	t.Parallel()

	first := kverr.AllReasons()
	require.NotEmpty(t, first)
	first[0] = kverr.Reason("MUTATED")

	assert.NotEqual(t, kverr.Reason("MUTATED"), kverr.AllReasons()[0])
}
