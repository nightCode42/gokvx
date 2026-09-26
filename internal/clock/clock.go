package clock

import "time"

// Clock tells the time and creates tickers.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// NewTicker returns a ticker that delivers ticks every d. Like
	// time.Ticker, it drops ticks for a slow receiver. It panics if d is
	// not positive, as time.NewTicker does.
	NewTicker(d time.Duration) Ticker
}

// Ticker delivers ticks at a fixed interval until stopped.
type Ticker interface {
	// C returns the channel on which ticks are delivered.
	C() <-chan time.Time
	// Stop turns the ticker off; no more ticks are delivered after it
	// returns.
	Stop()
}

// Real returns the Clock backed by the system clock.
func Real() Clock {
	return realClock{}
}

// realClock is the system clock.
type realClock struct{}

// Now returns the system time.
func (realClock) Now() time.Time {
	return time.Now()
}

// NewTicker returns a ticker backed by time.Ticker.
func (realClock) NewTicker(d time.Duration) Ticker {
	return realTicker{time.NewTicker(d)}
}

// realTicker adapts time.Ticker to the Ticker interface.
type realTicker struct {
	// ticker is the underlying system ticker.
	ticker *time.Ticker
}

// C returns the system ticker's channel.
func (t realTicker) C() <-chan time.Time {
	return t.ticker.C
}

// Stop stops the system ticker.
func (t realTicker) Stop() {
	t.ticker.Stop()
}
