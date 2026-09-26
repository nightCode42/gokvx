package storagetest

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sort"
	"sync"

	"github.com/nightCode42/gokvx/internal/kverr"
	"github.com/nightCode42/gokvx/internal/storage"
)

// MemEngine is an in-memory storage.Engine for tests. It keeps everything in
// a map and loses it on Close. It is safe for concurrent use.
type MemEngine struct {
	// mu guards data and closed.
	mu sync.Mutex
	// data maps each key to its value.
	data map[string][]byte
	// closed is set by Close.
	closed bool
}

// NewMemEngine returns an empty in-memory engine.
func NewMemEngine() *MemEngine {
	return &MemEngine{data: make(map[string][]byte)}
}

// errMemClosed is the cause reported after Close.
var errMemClosed = errors.New("closed")

// Get returns a copy of the value stored under key.
func (e *MemEngine) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.usable(ctx); err != nil {
		return nil, false, err
	}
	value, ok := e.data[string(key)]
	return slices.Clone(value), ok, nil
}

// Apply writes the batch atomically; sync has no effect in memory.
func (e *MemEngine) Apply(ctx context.Context, b *storage.Batch, _ bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.usable(ctx); err != nil {
		return err
	}
	for _, op := range b.Ops() {
		if op.Delete {
			delete(e.data, string(op.Key))
			continue
		}
		e.data[string(op.Key)] = slices.Clone(op.Value)
	}
	return nil
}

// NewIterator returns an iterator over a snapshot of the keys in
// [lower, upper).
func (e *MemEngine) NewIterator(ctx context.Context, lower, upper []byte) (storage.Iterator, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.usable(ctx); err != nil {
		return nil, err
	}

	var entries []memEntry
	for k, v := range e.data {
		key := []byte(k)
		if lower != nil && bytes.Compare(key, lower) < 0 {
			continue
		}
		if upper != nil && bytes.Compare(key, upper) >= 0 {
			continue
		}
		entries = append(entries, memEntry{key: key, value: slices.Clone(v)})
	}
	sort.Slice(entries, func(i, j int) bool { return bytes.Compare(entries[i].key, entries[j].key) < 0 })
	return &memIterator{entries: entries, pos: -1}, nil
}

// Close discards the data. Closing twice is harmless.
func (e *MemEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	e.data = nil
	return nil
}

// usable returns an error when the context is done or the engine is closed.
// The caller holds e.mu.
func (e *MemEngine) usable(ctx context.Context) error {
	if e.closed {
		return kverr.Wrap(kverr.ReasonInternal, errMemClosed, "storage engine is closed")
	}
	if err := ctx.Err(); err != nil {
		return kverr.Wrap(kverr.ReasonOf(err), err, "")
	}
	return nil
}

// memEntry is one key and value in an iterator's snapshot.
type memEntry struct {
	// key is the entry's key.
	key []byte
	// value is the entry's value.
	value []byte
}

// memIterator walks a sorted snapshot of entries.
type memIterator struct {
	// entries is the snapshot, in ascending key order.
	entries []memEntry
	// pos is the index of the current entry; -1 before the first.
	pos int
}

// Next moves to the next entry.
func (i *memIterator) Next() bool {
	if i.pos < len(i.entries) {
		i.pos++
	}
	return i.pos < len(i.entries)
}

// Key returns the current key.
func (i *memIterator) Key() []byte {
	return i.entries[i.pos].key
}

// Value returns the current value.
func (i *memIterator) Value() []byte {
	return i.entries[i.pos].value
}

// Err returns nil: an in-memory iteration cannot fail.
func (i *memIterator) Err() error {
	return nil
}

// Close releases the snapshot.
func (i *memIterator) Close() error {
	i.entries = nil
	return nil
}
