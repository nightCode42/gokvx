// Package clock abstracts the passage of time so that time-dependent
// behavior — fsync intervals, token refresh, lease expiry, election timing —
// can be tested deterministically (QA-003).
//
// Production code receives a [Clock] from [Real]; tests use the fake in
// package clocktest and advance it explicitly. Code that needs the time or a
// ticker takes a Clock as a constructor parameter and never calls the time
// package's Now or NewTicker directly.
package clock
