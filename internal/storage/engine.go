package storage

import (
	"context"
	"slices"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// Engine is an ordered key-value store with atomic batches: the layer the
// state machine persists its data in (KV-STO-001). Keys are ordered by
// unsigned byte comparison (KV-DAT-010).
//
// [Pebble] is the production implementation; storagetest.MemEngine is an
// in-memory implementation for tests. Both pass storagetest.RunEngineContract.
// An Engine is safe for concurrent use, except that Close must not run
// concurrently with any other method.
type Engine interface {
	// Get returns a copy of the value stored under key, and ok false when
	// there is none. An absent key is not an error.
	Get(ctx context.Context, key []byte) (value []byte, ok bool, err error)

	// Apply writes every operation in b atomically: after a crash, either
	// all of them are present or none is. With sync true, Apply returns only
	// once the writes are durable.
	Apply(ctx context.Context, b *Batch, sync bool) error

	// NewIterator returns an iterator over the keys in [lower, upper) in
	// ascending order. It sees the engine as it was when the iterator was
	// created. A nil bound leaves that side unbounded.
	NewIterator(ctx context.Context, lower, upper []byte) (Iterator, error)

	// Close releases the engine. Later calls to any method fail.
	Close() error
}

// Iterator walks keys in ascending order. It starts before the first key:
// call Next to move to it. An Iterator is not safe for concurrent use.
type Iterator interface {
	// Next moves to the next key and reports whether there is one. After
	// Next returns false, check Err.
	Next() bool
	// Key returns the current key. It is valid until the next call to Next;
	// copy it to keep it.
	Key() []byte
	// Value returns the current value, valid until the next call to Next.
	Value() []byte
	// Err returns the error that stopped the iteration, if any.
	Err() error
	// Close releases the iterator.
	Close() error
}

// Op is one write in a Batch.
type Op struct {
	// Key is the key written.
	Key []byte
	// Value is the value written; nil for a delete.
	Value []byte
	// Delete reports whether the op removes Key.
	Delete bool
}

// Batch collects writes to apply atomically with Engine.Apply. The zero value
// is an empty batch. A Batch is not safe for concurrent use.
type Batch struct {
	// ops holds the writes in the order they were added.
	ops []Op
}

// Set adds a write of value under key. Both are copied, so the caller may
// reuse them.
func (b *Batch) Set(key, value []byte) {
	b.ops = append(b.ops, Op{Key: slices.Clone(key), Value: slices.Clone(value)})
}

// Delete adds the removal of key. The key is copied.
func (b *Batch) Delete(key []byte) {
	b.ops = append(b.ops, Op{Key: slices.Clone(key), Delete: true})
}

// Len returns the number of writes in the batch.
func (b *Batch) Len() int {
	return len(b.ops)
}

// Ops returns the writes in the order they were added. Later writes to the
// same key win. The returned slice must not be modified.
func (b *Batch) Ops() []Op {
	return b.ops
}

// contextError returns the context's error as a kverr error, or nil while
// the context is live. Engine and Log methods check it on entry, so a
// canceled request stops before starting I/O.
func contextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return kverr.Wrap(kverr.ReasonOf(err), err, "")
	}
	return nil
}
