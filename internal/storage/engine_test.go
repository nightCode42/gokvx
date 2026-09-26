package storage_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/storage"
	"github.com/nightCode42/gokvx/internal/storage/storagetest"
)

// discardLogger returns a logger that drops everything.
func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// openPebble opens a Pebble engine in a temporary directory, closed when the
// test ends.
func openPebble(t *testing.T, dir string) *storage.Pebble {
	t.Helper()

	p, err := storage.OpenPebble(context.Background(), dir, discardLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// TestPebbleEngineContract_KV_STO_001 runs the Engine contract against Pebble.
// Verifies: KV-STO-001, KV-DAT-010.
func TestPebbleEngineContract_KV_STO_001(t *testing.T) {
	t.Parallel()

	storagetest.RunEngineContract(t, func(t *testing.T) storage.Engine {
		return openPebble(t, t.TempDir())
	})
}

// TestMemEngineContract_KV_STO_001 runs the Engine contract against the
// in-memory fake, so the fake cannot drift from Pebble's behavior.
// Verifies: KV-STO-001.
func TestMemEngineContract_KV_STO_001(t *testing.T) {
	t.Parallel()

	storagetest.RunEngineContract(t, func(*testing.T) storage.Engine {
		return storagetest.NewMemEngine()
	})
}

// TestPebbleSurvivesReopen checks that synced writes are still there after
// the engine is closed and reopened.
func TestPebbleSurvivesReopen(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx := context.Background()

	first := openPebble(t, dir)
	var b storage.Batch
	b.Set([]byte("k"), []byte("v"))
	require.NoError(t, first.Apply(ctx, &b, true))
	require.NoError(t, first.Close())

	second := openPebble(t, dir)
	value, ok, err := second.Get(ctx, []byte("k"))
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "v", string(value))
}

// TestPebbleLogsAsStructuredJSON checks that Pebble's own messages go through
// the node's logger rather than to stderr as plain text (KV-OBS-020).
func TestPebbleLogsAsStructuredJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	p, err := storage.OpenPebble(context.Background(), t.TempDir(), logger)
	require.NoError(t, err)
	require.NoError(t, p.Close())

	assert.Contains(t, buf.String(), `"component":"pebble"`)
}
