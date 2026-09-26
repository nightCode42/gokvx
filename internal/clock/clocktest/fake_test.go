package clocktest_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/nightCode42/gokvx/internal/clock"
	"github.com/nightCode42/gokvx/internal/clock/clocktest"
)

// start is an arbitrary fixed time for fake clocks.
var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// ticked reports whether a tick is waiting on c.
func ticked(c <-chan time.Time) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

// TestFakeNowMovesOnlyOnAdvance checks that fake time stands still until the
// test moves it.
func TestFakeNowMovesOnlyOnAdvance(t *testing.T) {
	t.Parallel()

	f := clocktest.New(start)
	assert.Equal(t, start, f.Now())

	f.Advance(90 * time.Second)

	assert.Equal(t, start.Add(90*time.Second), f.Now())
}

// TestFakeTickerFiresWhenDue checks that a ticker fires once its interval has
// passed, and not before.
func TestFakeTickerFiresWhenDue(t *testing.T) {
	t.Parallel()

	f := clocktest.New(start)
	tk := f.NewTicker(time.Second)

	f.Advance(999 * time.Millisecond)
	assert.False(t, ticked(tk.C()), "not yet due")

	f.Advance(time.Millisecond)
	assert.True(t, ticked(tk.C()), "due")
	assert.False(t, ticked(tk.C()), "one tick per interval")
}

// TestFakeTickerDropsTicksForSlowReceiver checks that, like time.Ticker, a
// ticker whose tick is unread does not queue more.
func TestFakeTickerDropsTicksForSlowReceiver(t *testing.T) {
	t.Parallel()

	f := clocktest.New(start)
	tk := f.NewTicker(time.Second)

	f.Advance(5 * time.Second)
	f.Advance(time.Second)

	assert.True(t, ticked(tk.C()))
	assert.False(t, ticked(tk.C()))
}

// TestFakeTickerStop checks that a stopped ticker stays silent.
func TestFakeTickerStop(t *testing.T) {
	t.Parallel()

	f := clocktest.New(start)
	tk := f.NewTicker(time.Second)
	tk.Stop()

	f.Advance(time.Minute)

	assert.False(t, ticked(tk.C()))
}

// TestFakeTickerRejectsNonPositiveInterval matches time.NewTicker.
func TestFakeTickerRejectsNonPositiveInterval(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() { clocktest.New(start).NewTicker(0) })
}

// TestFakeImplementsClock keeps the fake interchangeable with the real clock.
func TestFakeImplementsClock(t *testing.T) {
	t.Parallel()

	var c clock.Clock = clocktest.New(start)

	assert.Equal(t, start, c.Now())
}
