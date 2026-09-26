package clocktest

import (
	"sync"
	"time"

	"github.com/nightCode42/gokvx/internal/clock"
)

// Fake is a clock.Clock whose time changes only through Advance. It is safe
// for concurrent use.
type Fake struct {
	// mu guards now and tickers.
	mu sync.Mutex
	// now is the current fake time.
	now time.Time
	// tickers holds the tickers that have not been stopped.
	tickers map[*fakeTicker]struct{}
}

// New returns a fake clock set to start.
func New(start time.Time) *Fake {
	return &Fake{now: start, tickers: make(map[*fakeTicker]struct{})}
}

// Now returns the fake time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// NewTicker returns a ticker that fires as Advance moves the fake time past
// each multiple of d. It panics if d is not positive, as time.NewTicker does.
func (f *Fake) NewTicker(d time.Duration) clock.Ticker {
	if d <= 0 {
		panic("clocktest: non-positive interval for NewTicker")
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	t := &fakeTicker{fake: f, interval: d, next: f.now.Add(d), c: make(chan time.Time, 1)}
	f.tickers[t] = struct{}{}
	return t
}

// Advance moves the fake time forward by d and fires every ticker that is
// due. A ticker due several times fires once, as a slow receiver of a
// time.Ticker would see.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = f.now.Add(d)
	for t := range f.tickers {
		if f.now.Before(t.next) {
			continue
		}
		select {
		case t.c <- f.now:
		default: // the previous tick is unread; drop this one
		}
		for !f.now.Before(t.next) {
			t.next = t.next.Add(t.interval)
		}
	}
}

// fakeTicker is a ticker driven by a Fake.
type fakeTicker struct {
	// fake is the clock the ticker belongs to.
	fake *Fake
	// interval is the time between ticks.
	interval time.Duration
	// next is the fake time of the next tick; guarded by fake.mu.
	next time.Time
	// c delivers ticks; buffered by one, like time.Ticker.
	c chan time.Time
}

// C returns the channel on which ticks are delivered.
func (t *fakeTicker) C() <-chan time.Time {
	return t.c
}

// Stop removes the ticker from its clock; no ticks follow.
func (t *fakeTicker) Stop() {
	t.fake.mu.Lock()
	defer t.fake.mu.Unlock()
	delete(t.fake.tickers, t)
}
