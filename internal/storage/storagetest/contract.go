package storagetest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/kverr"
	"github.com/nightCode42/gokvx/internal/storage"
)

// RunEngineContract runs the behavior every storage.Engine must provide
// against engines built by open. Each subtest gets a fresh engine, which the
// subtest closes.
func RunEngineContract(t *testing.T, open func(t *testing.T) storage.Engine) {
	t.Helper()

	tests := map[string]func(t *testing.T, e storage.Engine){
		"missing key is not an error":      contractMissingKey,
		"set, overwrite, delete":           contractSetOverwriteDelete,
		"batch applies every op":           contractBatch,
		"later op in a batch wins":         contractLastOpWins,
		"values are copies":                contractValuesAreCopies,
		"iterator orders unsigned bytes":   contractUnsignedOrder,
		"iterator respects bounds":         contractBounds,
		"iterator sees a snapshot":         contractSnapshot,
		"empty iteration":                  contractEmptyIteration,
		"canceled context stops I/O":       contractCanceledContext,
		"closed engine refuses operations": contractClosed,
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			e := open(t)
			test(t, e)
		})
	}
}

// ctx is the background context used by the contract tests.
var ctx = context.Background()

// set writes one key through a batch.
func set(t *testing.T, e storage.Engine, key, value string) {
	t.Helper()

	var b storage.Batch
	b.Set([]byte(key), []byte(value))
	require.NoError(t, e.Apply(ctx, &b, true))
}

// get reads one key, failing the test on error.
func get(t *testing.T, e storage.Engine, key string) (string, bool) {
	t.Helper()

	value, ok, err := e.Get(ctx, []byte(key))
	require.NoError(t, err)
	return string(value), ok
}

// scan returns the keys in [lower, upper), in iteration order.
func scan(t *testing.T, e storage.Engine, lower, upper []byte) []string {
	t.Helper()

	it, err := e.NewIterator(ctx, lower, upper)
	require.NoError(t, err)
	var keys []string
	for it.Next() {
		keys = append(keys, string(it.Key()))
	}
	require.NoError(t, it.Err())
	require.NoError(t, it.Close())
	return keys
}

// contractMissingKey checks that an absent key reports ok false, not an error.
func contractMissingKey(t *testing.T, e storage.Engine) {
	_, ok := get(t, e, "absent")
	assert.False(t, ok)
}

// contractSetOverwriteDelete checks the basic write operations.
func contractSetOverwriteDelete(t *testing.T, e storage.Engine) {
	set(t, e, "k", "v1")
	value, ok := get(t, e, "k")
	assert.True(t, ok)
	assert.Equal(t, "v1", value)

	set(t, e, "k", "v2")
	value, _ = get(t, e, "k")
	assert.Equal(t, "v2", value)

	var b storage.Batch
	b.Delete([]byte("k"))
	require.NoError(t, e.Apply(ctx, &b, true))
	_, ok = get(t, e, "k")
	assert.False(t, ok)
}

// contractBatch checks that every operation of a batch is applied.
func contractBatch(t *testing.T, e storage.Engine) {
	set(t, e, "gone", "x")

	var b storage.Batch
	b.Set([]byte("a"), []byte("1"))
	b.Set([]byte("b"), []byte("2"))
	b.Delete([]byte("gone"))
	assert.Equal(t, 3, b.Len())
	require.NoError(t, e.Apply(ctx, &b, false))

	assert.Equal(t, []string{"a", "b"}, scan(t, e, nil, nil))
}

// contractLastOpWins checks that the last write to a key within one batch
// decides its value.
func contractLastOpWins(t *testing.T, e storage.Engine) {
	var b storage.Batch
	b.Set([]byte("k"), []byte("first"))
	b.Delete([]byte("k"))
	b.Set([]byte("k"), []byte("last"))
	require.NoError(t, e.Apply(ctx, &b, true))

	value, _ := get(t, e, "k")
	assert.Equal(t, "last", value)
}

// contractValuesAreCopies checks that neither the caller's buffers nor the
// returned values alias the engine's data.
func contractValuesAreCopies(t *testing.T, e storage.Engine) {
	key, value := []byte("k"), []byte("original")
	var b storage.Batch
	b.Set(key, value)
	value[0] = 'X'
	key[0] = 'X'
	require.NoError(t, e.Apply(ctx, &b, true))

	got, ok, err := e.Get(ctx, []byte("k"))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "original", string(got))

	got[0] = 'Y'
	again, _ := get(t, e, "k")
	assert.Equal(t, "original", again)
}

// contractUnsignedOrder checks that keys sort as unsigned bytes (KV-DAT-010).
func contractUnsignedOrder(t *testing.T, e storage.Engine) {
	for _, k := range []string{"\xff", "b", "\x00", "a", "\x80", "ab"} {
		set(t, e, k, "v")
	}

	assert.Equal(t, []string{"\x00", "a", "ab", "b", "\x80", "\xff"}, scan(t, e, nil, nil))
}

// contractBounds checks that iteration covers exactly [lower, upper).
func contractBounds(t *testing.T, e storage.Engine) {
	for _, k := range []string{"a", "b", "c", "d"} {
		set(t, e, k, "v")
	}

	assert.Equal(t, []string{"b", "c"}, scan(t, e, []byte("b"), []byte("d")))
	assert.Equal(t, []string{"c", "d"}, scan(t, e, []byte("c"), nil))
	assert.Equal(t, []string{"a"}, scan(t, e, nil, []byte("b")))
	assert.Empty(t, scan(t, e, []byte("b"), []byte("b")))
}

// contractSnapshot checks that an iterator does not see writes made after it
// was created.
func contractSnapshot(t *testing.T, e storage.Engine) {
	set(t, e, "a", "v")
	it, err := e.NewIterator(ctx, nil, nil)
	require.NoError(t, err)

	set(t, e, "b", "v")

	var keys []string
	for it.Next() {
		keys = append(keys, string(it.Key()))
		assert.Equal(t, "v", string(it.Value()))
	}
	require.NoError(t, it.Err())
	require.NoError(t, it.Close())
	assert.Equal(t, []string{"a"}, keys)
}

// contractEmptyIteration checks an iterator over an empty engine.
func contractEmptyIteration(t *testing.T, e storage.Engine) {
	assert.Empty(t, scan(t, e, nil, nil))
}

// contractCanceledContext checks that a canceled request fails with
// CANCELED before doing any I/O.
func contractCanceledContext(t *testing.T, e storage.Engine) {
	canceled, cancel := context.WithCancel(ctx)
	cancel()

	_, _, err := e.Get(canceled, []byte("k"))
	assert.Equal(t, kverr.ReasonCanceled, kverr.ReasonOf(err))
	err = e.Apply(canceled, &storage.Batch{}, true)
	assert.Equal(t, kverr.ReasonCanceled, kverr.ReasonOf(err))
	_, err = e.NewIterator(canceled, nil, nil)
	assert.Equal(t, kverr.ReasonCanceled, kverr.ReasonOf(err))
}

// contractClosed checks that a closed engine returns errors instead of
// panicking, and that closing twice is harmless.
func contractClosed(t *testing.T, e storage.Engine) {
	require.NoError(t, e.Close())
	require.NoError(t, e.Close())

	_, _, err := e.Get(ctx, []byte("k"))
	require.Error(t, err)
	require.Error(t, e.Apply(ctx, &storage.Batch{}, true))
	_, err = e.NewIterator(ctx, nil, nil)
	require.Error(t, err)
}
