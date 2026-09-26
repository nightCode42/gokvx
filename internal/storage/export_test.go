package storage

// Test-only access to internals, compiled only into this package's tests.

// SetOnSync makes l call f after every fsync of its active segment.
func SetOnSync(l *Log, f func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onSync = f
}

// FailLog puts l in the state a failed write or fsync leaves it in, since a
// real disk failure cannot be induced portably.
func FailLog(l *Log, cause error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failed = cause
}

// EncodeRecord exposes the record encoding, for tests that build damaged logs.
var EncodeRecord = encodeRecord

// MaxCommandSize exposes the largest command a record can hold.
const MaxCommandSize = maxCommandSize
