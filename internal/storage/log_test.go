package storage_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/clock"
	"github.com/nightCode42/gokvx/internal/clock/clocktest"
	"github.com/nightCode42/gokvx/internal/config"
	"github.com/nightCode42/gokvx/internal/kverr"
	"github.com/nightCode42/gokvx/internal/storage"
)

// logConfig returns a configuration for a log in dir with the always policy
// and small segments, so tests cross segment boundaries.
func logConfig(dir string) storage.LogConfig {
	return storage.LogConfig{Dir: dir, SegmentBytes: 256, Fsync: config.FsyncAlways}
}

// openLog opens a log, failing the test on error; the test closes it.
func openLog(t *testing.T, cfg storage.LogConfig, clk clock.Clock) *storage.Log {
	t.Helper()

	l, err := storage.OpenLog(context.Background(), cfg, clk, discardLogger())
	require.NoError(t, err)
	return l
}

// command returns the deterministic command stored at index i. Lengths vary
// so records land at every position within a segment.
func command(i uint64) []byte {
	return []byte(fmt.Sprintf("cmd-%d-%s", i, strings.Repeat("x", int(i%37))))
}

// appendN appends the commands for indexes last+1 through last+n.
func appendN(t *testing.T, l *storage.Log, n int) {
	t.Helper()

	for range n {
		want := l.LastIndex() + 1
		got, err := l.Append(context.Background(), command(want))
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

// replayAll returns every record from index from, checking each command.
func replayAll(t *testing.T, l *storage.Log, from uint64) []uint64 {
	t.Helper()

	var indexes []uint64
	err := l.Replay(context.Background(), from, func(index uint64, cmd []byte) error {
		assert.Equal(t, string(command(index)), string(cmd), "record %d", index)
		indexes = append(indexes, index)
		return nil
	})
	require.NoError(t, err)
	return indexes
}

// indexes returns the sequence from..to.
func indexes(from, to uint64) []uint64 {
	var out []uint64
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

// TestLogAppendAndReplay_KV_STO_002 checks that records come back in order,
// numbered from 1, from any starting index.
// Verifies: KV-STO-002.
func TestLogAppendAndReplay_KV_STO_002(t *testing.T) {
	t.Parallel()

	l := openLog(t, logConfig(t.TempDir()), clock.Real())
	defer func() { require.NoError(t, l.Close()) }()
	assert.Zero(t, l.LastIndex())

	appendN(t, l, 25)

	assert.Equal(t, uint64(25), l.LastIndex())
	assert.Equal(t, indexes(1, 25), replayAll(t, l, 1))
	assert.Equal(t, indexes(18, 25), replayAll(t, l, 18))
	assert.Empty(t, replayAll(t, l, 26))
}

// TestLogReopenContinues_KV_STO_003 checks that a reopened log holds every
// record and continues numbering where it stopped.
// Verifies: KV-STO-003.
func TestLogReopenContinues_KV_STO_003(t *testing.T) {
	t.Parallel()

	cfg := logConfig(t.TempDir())
	first := openLog(t, cfg, clock.Real())
	appendN(t, first, 12)
	require.NoError(t, first.Close())

	second := openLog(t, cfg, clock.Real())
	defer func() { require.NoError(t, second.Close()) }()
	assert.Equal(t, uint64(12), second.LastIndex())

	appendN(t, second, 3)
	assert.Equal(t, indexes(1, 15), replayAll(t, second, 1))
}

// TestLogRotatesSegments_KV_STO_002 checks that the log starts a new segment
// when one fills, names segments after their first index, and gives a record
// larger than a segment a segment of its own.
// Verifies: KV-STO-002.
func TestLogRotatesSegments_KV_STO_002(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	l := openLog(t, logConfig(dir), clock.Real())
	appendN(t, l, 20)
	big, err := l.Append(context.Background(), []byte(strings.Repeat("B", 1000)))
	require.NoError(t, err)
	_, err = l.Append(context.Background(), []byte("after"))
	require.NoError(t, err)
	require.NoError(t, l.Close())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Greater(t, len(entries), 3, "several segments")
	assert.Equal(t, "00000000000000000001.log", entries[0].Name())

	info, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%020d.log", big)))
	require.NoError(t, err, "the oversized record starts its own segment")
	assert.Greater(t, info.Size(), int64(1000))
}

// syncCounter counts fsyncs of a log's active segment and signals each one.
type syncCounter struct {
	count atomic.Int64
	ch    chan struct{}
}

// watchSyncs attaches a counter to l.
func watchSyncs(l *storage.Log) *syncCounter {
	c := &syncCounter{ch: make(chan struct{}, 100)}
	storage.SetOnSync(l, func() {
		c.count.Add(1)
		c.ch <- struct{}{}
	})
	return c
}

// awaitSync waits for the next fsync, failing after a generous timeout. The
// timeout only guards against a hang; correctness never depends on it.
func (c *syncCounter) awaitSync(t *testing.T) {
	t.Helper()

	select {
	case <-c.ch:
	case <-time.After(10 * time.Second):
		require.Fail(t, "no fsync within 10s")
	}
}

// TestLogFsyncAlways_KV_STO_005 checks that with the default policy every
// record is flushed before Append returns.
// Verifies: KV-STO-005.
func TestLogFsyncAlways_KV_STO_005(t *testing.T) {
	t.Parallel()

	cfg := logConfig(t.TempDir())
	cfg.SegmentBytes = 1 << 20
	l := openLog(t, cfg, clock.Real())
	syncs := watchSyncs(l)

	appendN(t, l, 5)
	assert.Equal(t, int64(5), syncs.count.Load(), "one fsync per acknowledged record")

	require.NoError(t, l.Close())
	assert.Equal(t, int64(5), syncs.count.Load(), "nothing left to flush on close")
}

// TestLogFsyncInterval_KV_STO_005 checks that with the interval policy,
// records are flushed by the background syncer on each tick, not on append.
// Verifies: KV-STO-005.
func TestLogFsyncInterval_KV_STO_005(t *testing.T) {
	t.Parallel()

	fake := clocktest.New(time.Unix(0, 0))
	cfg := logConfig(t.TempDir())
	cfg.SegmentBytes = 1 << 20
	cfg.Fsync, cfg.FsyncInterval = config.FsyncInterval, 100*time.Millisecond
	l := openLog(t, cfg, fake)
	syncs := watchSyncs(l)

	appendN(t, l, 3)
	assert.Zero(t, syncs.count.Load(), "appends do not flush")

	fake.Advance(100 * time.Millisecond)
	syncs.awaitSync(t)

	fake.Advance(100 * time.Millisecond) // nothing new to flush
	require.NoError(t, l.Close())
	assert.Equal(t, int64(1), syncs.count.Load())
}

// TestLogFsyncOS_KV_STO_005 checks that with the os policy the log never
// flushes on its own, but still flushes on a clean close.
// Verifies: KV-STO-005.
func TestLogFsyncOS_KV_STO_005(t *testing.T) {
	t.Parallel()

	cfg := logConfig(t.TempDir())
	cfg.SegmentBytes = 1 << 20
	cfg.Fsync = config.FsyncOS
	l := openLog(t, cfg, clock.Real())
	syncs := watchSyncs(l)

	appendN(t, l, 5)
	assert.Zero(t, syncs.count.Load())

	require.NoError(t, l.Close())
	assert.Equal(t, int64(1), syncs.count.Load())
}

// TestLogRejectsOversizedCommand checks the record size limit.
func TestLogRejectsOversizedCommand(t *testing.T) {
	t.Parallel()

	l := openLog(t, logConfig(t.TempDir()), clock.Real())
	defer func() { require.NoError(t, l.Close()) }()

	_, err := l.Append(context.Background(), make([]byte, storage.MaxCommandSize+1))

	assert.Equal(t, kverr.ReasonInvalidArgument, kverr.ReasonOf(err))
	assert.Zero(t, l.LastIndex())
}

// TestLogRefusesWritesAfterFailure_KV_STO_005 checks that after a failed
// write or fsync the log refuses every later append: the kernel may have
// dropped the unflushed data, so acknowledging anything further would be a
// lie. The failure is reported as the cause.
// Verifies: KV-STO-005.
func TestLogRefusesWritesAfterFailure_KV_STO_005(t *testing.T) {
	t.Parallel()

	l := openLog(t, logConfig(t.TempDir()), clock.Real())
	appendN(t, l, 2)
	cause := errors.New("fsync: input/output error")
	storage.FailLog(l, cause)

	_, err := l.Append(context.Background(), []byte("after failure"))

	assert.ErrorIs(t, err, cause)
	assert.Equal(t, uint64(2), l.LastIndex())
	require.NoError(t, l.Close(), "close skips the flush of an already failed log")
}

// TestLogClosed checks that a closed log refuses appends and that closing
// twice is harmless.
func TestLogClosed(t *testing.T) {
	t.Parallel()

	l := openLog(t, logConfig(t.TempDir()), clock.Real())
	require.NoError(t, l.Close())
	require.NoError(t, l.Close())

	_, err := l.Append(context.Background(), []byte("late"))
	assert.Error(t, err)
}

// TestLogHonorsCanceledContext checks that canceled requests stop before any
// I/O.
func TestLogHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := storage.OpenLog(ctx, logConfig(t.TempDir()), clock.Real(), discardLogger())
	assert.Equal(t, kverr.ReasonCanceled, kverr.ReasonOf(err))

	l := openLog(t, logConfig(t.TempDir()), clock.Real())
	defer func() { require.NoError(t, l.Close()) }()
	appendN(t, l, 3)

	_, err = l.Append(ctx, []byte("x"))
	assert.Equal(t, kverr.ReasonCanceled, kverr.ReasonOf(err))
	err = l.Replay(ctx, 1, func(uint64, []byte) error { return nil })
	assert.Equal(t, kverr.ReasonCanceled, kverr.ReasonOf(err))
}

// TestLogReplayStopsAtCallbackError checks that an error from the callback
// ends the replay and is returned.
func TestLogReplayStopsAtCallbackError(t *testing.T) {
	t.Parallel()

	l := openLog(t, logConfig(t.TempDir()), clock.Real())
	defer func() { require.NoError(t, l.Close()) }()
	appendN(t, l, 5)

	stop := kverr.New(kverr.ReasonInternal, "stop here")
	var seen []uint64
	err := l.Replay(context.Background(), 1, func(index uint64, _ []byte) error {
		seen = append(seen, index)
		if index == 2 {
			return stop
		}
		return nil
	})

	assert.ErrorIs(t, err, stop)
	assert.Equal(t, []uint64{1, 2}, seen)
}

// TestLogConfigValidate checks every configuration rule.
func TestLogConfigValidate(t *testing.T) {
	t.Parallel()

	valid := logConfig("dir")
	tests := []struct {
		name   string
		mutate func(c *storage.LogConfig)
		field  string
	}{
		{"no directory", func(c *storage.LogConfig) { c.Dir = "" }, "Dir"},
		{"zero segment size", func(c *storage.LogConfig) { c.SegmentBytes = 0 }, "SegmentBytes"},
		{"unknown policy", func(c *storage.LogConfig) { c.Fsync = "never" }, "Fsync"},
		{"interval without period", func(c *storage.LogConfig) { c.Fsync = config.FsyncInterval }, "FsyncInterval"},
	}

	require.NoError(t, valid.Validate())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := valid
			tt.mutate(&cfg)

			var kerr *kverr.Error
			require.ErrorAs(t, cfg.Validate(), &kerr)
			require.Len(t, kerr.Violations(), 1)
			assert.Equal(t, tt.field, kerr.Violations()[0].Field)
		})
	}
}
