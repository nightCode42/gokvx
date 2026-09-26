package clock_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/clock"
)

// TestRealClockNow checks that the real clock reads the system time.
func TestRealClockNow(t *testing.T) {
	t.Parallel()

	before := time.Now()
	now := clock.Real().Now()

	assert.False(t, now.Before(before))
}

// TestRealTickerTicks checks that the real ticker delivers a tick and stops.
// It waits for one short real interval, bounded by a generous timeout.
func TestRealTickerTicks(t *testing.T) {
	t.Parallel()

	tk := clock.Real().NewTicker(time.Millisecond)
	defer tk.Stop()

	select {
	case <-tk.C():
	case <-time.After(5 * time.Second):
		require.Fail(t, "no tick within 5s")
	}
}
