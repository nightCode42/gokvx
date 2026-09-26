package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync/atomic"

	"github.com/cockroachdb/pebble/v2"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// pebbleFormat is the Pebble on-disk format version used to create stores.
// It is pinned rather than left to Pebble's default, so that a format change
// only ever happens deliberately. Raising it upgrades existing stores one
// way: an older binary can no longer open them.
const pebbleFormat = pebble.FormatValueSeparation

// errClosed is the cause reported by an engine or log used after Close.
var errClosed = errors.New("closed")

// Pebble is the Engine implementation backed by a Pebble database
// (KV-STO-001, ADR-0005).
type Pebble struct {
	// db is the underlying database.
	db *pebble.DB
	// closed is set by Close; later calls fail instead of reaching a closed
	// database, which would panic.
	closed atomic.Bool
}

// OpenPebble opens, or creates, the Pebble database in dir. Pebble's own log
// messages are written to logger: routine events at DEBUG, errors at ERROR.
func OpenPebble(ctx context.Context, dir string, logger *slog.Logger) (*Pebble, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	db, err := pebble.Open(dir, &pebble.Options{
		FormatMajorVersion: pebbleFormat,
		Logger:             pebbleLogger{logger: logger},
	})
	if err != nil {
		return nil, kverr.Wrap(kverr.ReasonInternal, err, "storage engine could not be opened")
	}
	return &Pebble{db: db}, nil
}

// Get returns a copy of the value stored under key.
func (p *Pebble) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if err := p.usable(ctx); err != nil {
		return nil, false, err
	}
	value, closer, err := p.db.Get(key)
	if errors.Is(err, pebble.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, kverr.Wrap(kverr.ReasonInternal, err, "storage engine read failed")
	}
	// Pebble's value is valid only until the closer is closed.
	value = slices.Clone(value)
	if err := closer.Close(); err != nil {
		return nil, false, kverr.Wrap(kverr.ReasonInternal, err, "storage engine read failed")
	}
	return value, true, nil
}

// Apply writes the batch atomically, durably when sync is true.
func (p *Pebble) Apply(ctx context.Context, b *Batch, sync bool) error {
	if err := p.usable(ctx); err != nil {
		return err
	}
	pb := p.db.NewBatch()
	defer func() { _ = pb.Close() }()

	for _, op := range b.Ops() {
		var err error
		if op.Delete {
			err = pb.Delete(op.Key, nil)
		} else {
			err = pb.Set(op.Key, op.Value, nil)
		}
		if err != nil {
			return kverr.Wrap(kverr.ReasonInternal, err, "storage engine batch could not be built")
		}
	}

	opts := pebble.NoSync
	if sync {
		opts = pebble.Sync
	}
	if err := pb.Commit(opts); err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "storage engine write failed")
	}
	return nil
}

// NewIterator returns an iterator over [lower, upper) that sees a
// consistent snapshot of the database.
func (p *Pebble) NewIterator(ctx context.Context, lower, upper []byte) (Iterator, error) {
	if err := p.usable(ctx); err != nil {
		return nil, err
	}
	// Pebble requires the bounds to stay unchanged for the iterator's life.
	it, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: slices.Clone(lower),
		UpperBound: slices.Clone(upper),
	})
	if err != nil {
		return nil, kverr.Wrap(kverr.ReasonInternal, err, "storage engine iterator could not be created")
	}
	return &pebbleIterator{it: it}, nil
}

// Close closes the database. Closing twice is harmless.
func (p *Pebble) Close() error {
	if p.closed.Swap(true) {
		return nil
	}
	if err := p.db.Close(); err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "storage engine could not be closed")
	}
	return nil
}

// usable returns an error when the context is done or the engine is closed.
func (p *Pebble) usable(ctx context.Context) error {
	if p.closed.Load() {
		return kverr.Wrap(kverr.ReasonInternal, errClosed, "storage engine is closed")
	}
	return contextError(ctx)
}

// pebbleIterator adapts a Pebble iterator to the Iterator interface.
type pebbleIterator struct {
	// it is the underlying iterator.
	it *pebble.Iterator
	// started records whether Next has positioned the iterator yet.
	started bool
	// value is the current value, read when the iterator moves.
	value []byte
	// err is the first error met while reading a value.
	err error
}

// Next moves to the first key on its first call, and to the following key
// afterwards.
func (i *pebbleIterator) Next() bool {
	var ok bool
	if i.started {
		ok = i.it.Next()
	} else {
		i.started = true
		ok = i.it.First()
	}
	if !ok {
		return false
	}
	// Pebble may store large values separately; ValueAndErr reports a
	// failure to fetch one, which Value alone would hide.
	value, err := i.it.ValueAndErr()
	if err != nil {
		i.err = err
		return false
	}
	i.value = value
	return true
}

// Key returns the current key.
func (i *pebbleIterator) Key() []byte {
	return i.it.Key()
}

// Value returns the current value.
func (i *pebbleIterator) Value() []byte {
	return i.value
}

// Err returns the error that ended the iteration, if any.
func (i *pebbleIterator) Err() error {
	err := i.err
	if err == nil {
		err = i.it.Error()
	}
	if err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "storage engine iteration failed")
	}
	return nil
}

// Close releases the iterator.
func (i *pebbleIterator) Close() error {
	if err := i.it.Close(); err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "storage engine iterator could not be closed")
	}
	return nil
}

// pebbleLogger routes Pebble's log messages to the node's structured logger,
// so every line the process writes is JSON (KV-OBS-020). Pebble reports
// flushes and compactions through Infof; they are frequent, so they are
// logged at DEBUG (KV-OBS-023).
type pebbleLogger struct {
	// logger receives the messages.
	logger *slog.Logger
}

// Infof logs a routine Pebble event at DEBUG.
func (l pebbleLogger) Infof(format string, args ...any) {
	l.logger.Debug(fmt.Sprintf(format, args...), slog.String("component", "pebble"))
}

// Errorf logs a Pebble error at ERROR.
func (l pebbleLogger) Errorf(format string, args ...any) {
	l.logger.Error(fmt.Sprintf(format, args...), slog.String("component", "pebble"))
}

// Fatalf logs an unrecoverable Pebble failure and exits. Pebble calls it
// only when the database cannot continue safely, and its contract requires
// that Fatalf does not return; exiting matches Pebble's default logger.
func (l pebbleLogger) Fatalf(format string, args ...any) {
	l.logger.Error(fmt.Sprintf(format, args...), slog.String("component", "pebble"), slog.Bool("fatal", true))
	os.Exit(1)
}
