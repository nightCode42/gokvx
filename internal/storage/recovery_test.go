package storage_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/clock"
	"github.com/nightCode42/gokvx/internal/storage"
)

// recordSize returns the on-disk size of the record for index i.
func recordSize(i uint64) int {
	return 16 + len(command(i))
}

// singleSegment writes n records into one segment in a new directory and
// returns the directory and the segment's path.
func singleSegment(t *testing.T, n int) (string, string) {
	t.Helper()

	dir := t.TempDir()
	cfg := logConfig(dir)
	cfg.SegmentBytes = 1 << 20
	l := openLog(t, cfg, clock.Real())
	appendN(t, l, n)
	require.NoError(t, l.Close())
	return dir, filepath.Join(dir, "00000000000000000001.log")
}

// openCapturing opens the log in dir with a logger whose output is returned.
func openCapturing(t *testing.T, dir string) (*storage.Log, *bytes.Buffer, error) {
	t.Helper()

	var buf bytes.Buffer
	cfg := logConfig(dir)
	cfg.SegmentBytes = 1 << 20
	l, err := storage.OpenLog(context.Background(), cfg, clock.Real(), slog.New(slog.NewJSONHandler(&buf, nil)))
	return l, &buf, err //nolint:wrapcheck // returned for the caller to inspect.
}

// flipByte inverts one byte of a file.
func flipByte(t *testing.T, path string, offset int) {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // a segment in the test's temporary directory.
	require.NoError(t, err)
	data[offset] ^= 0xFF
	require.NoError(t, os.WriteFile(path, data, 0o600)) //nolint:gosec // the test deliberately damages its own file.
}

// TestTornWriteRecovery_KV_STO_021 cuts the final record at every possible
// byte offset, as a crash in the middle of a write would, and checks that the
// log recovers every earlier record, truncates the partial one with a
// warning naming its offset, and accepts new records.
// Verifies: KV-STO-021, KV-STO-004.
func TestTornWriteRecovery_KV_STO_021(t *testing.T) {
	t.Parallel()

	const n = 5
	_, template := singleSegment(t, n)
	original, err := os.ReadFile(template) //nolint:gosec // a segment in the test's temporary directory.
	require.NoError(t, err)
	lastStart := len(original) - recordSize(n)

	for cut := lastStart + 1; cut < len(original); cut++ {
		dir := t.TempDir()
		path := filepath.Join(dir, "00000000000000000001.log")
		require.NoError(t, os.WriteFile(path, original[:cut], 0o600)) //nolint:gosec // the test deliberately writes a torn segment.

		l, logs, err := openCapturing(t, dir)
		require.NoError(t, err, "cut at %d", cut)
		assert.Equal(t, uint64(n-1), l.LastIndex(), "cut at %d", cut)
		assert.Equal(t, indexes(1, n-1), replayAll(t, l, 1))
		assert.Contains(t, logs.String(), `"level":"WARN"`)
		assert.Contains(t, logs.String(), `"offset":`+strconv.Itoa(lastStart))

		appendN(t, l, 1)
		assert.Equal(t, indexes(1, n), replayAll(t, l, 1), "a new record replaces the torn one")
		require.NoError(t, l.Close())

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, int64(len(original)), info.Size(), "cut at %d", cut)
	}
}

// TestCorruptTailIsDetected_KV_STO_022 flips bytes in the final record and
// checks that the damage is detected and truncated with a warning, never
// applied.
// Verifies: KV-STO-022, KV-STO-004.
func TestCorruptTailIsDetected_KV_STO_022(t *testing.T) {
	t.Parallel()

	const n = 4
	for _, where := range []struct {
		name   string
		offset func(lastStart int) int
	}{
		{"command byte", func(s int) int { return s + recordSize(n) - 1 }},
		{"index byte", func(s int) int { return s + 8 }},
		{"checksum byte", func(s int) int { return s + 5 }},
		{"length byte", func(s int) int { return s }},
	} {
		t.Run(where.name, func(t *testing.T) {
			t.Parallel()

			dir, path := singleSegment(t, n)
			info, err := os.Stat(path)
			require.NoError(t, err)
			flipByte(t, path, where.offset(int(info.Size())-recordSize(n)))

			l, logs, err := openCapturing(t, dir)
			require.NoError(t, err)
			defer func() { require.NoError(t, l.Close()) }()

			assert.Equal(t, indexes(1, n-1), replayAll(t, l, 1))
			assert.Contains(t, logs.String(), `"level":"WARN"`)
		})
	}
}

// TestMidLogCorruptionRefusesToOpen_KV_STO_004 damages a record that intact
// records follow, and checks that the log refuses to open rather than
// silently dropping them — including when the damage hides where the next
// record starts.
// Verifies: KV-STO-004, KV-STO-022.
func TestMidLogCorruptionRefusesToOpen_KV_STO_004(t *testing.T) {
	t.Parallel()

	const n = 6
	secondStart := recordSize(1)
	for _, where := range []struct {
		name   string
		offset int
	}{
		{"command byte of record 2", secondStart + recordSize(2) - 1},
		{"checksum of record 2", secondStart + 4},
		{"length of record 2", secondStart + 3},
	} {
		t.Run(where.name, func(t *testing.T) {
			t.Parallel()

			dir, path := singleSegment(t, n)
			flipByte(t, path, where.offset)

			_, _, err := openCapturing(t, dir)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "corrupted")
		})
	}
}

// TestCorruptionInEarlierSegmentRefusesToOpen_KV_STO_004 checks that damage
// in any segment but the last is never taken for a torn tail.
// Verifies: KV-STO-004.
func TestCorruptionInEarlierSegmentRefusesToOpen_KV_STO_004(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	l := openLog(t, logConfig(dir), clock.Real())
	appendN(t, l, 30)
	require.NoError(t, l.Close())

	first := filepath.Join(dir, "00000000000000000001.log")
	info, err := os.Stat(first)
	require.NoError(t, err)
	flipByte(t, first, int(info.Size())-1) // the first segment's last byte

	_, err = storage.OpenLog(context.Background(), logConfig(dir), clock.Real(), discardLogger())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "corrupted")
}

// writeRecords writes a segment file from records built with EncodeRecord.
func writeRecords(t *testing.T, path string, indexes ...uint64) {
	t.Helper()

	var data []byte
	for _, i := range indexes {
		data = append(data, storage.EncodeRecord(i, command(i))...)
	}
	require.NoError(t, os.WriteFile(path, data, 0o600)) //nolint:gosec // the test deliberately damages its own file.
}

// TestOutOfSequenceRecordRefusesToOpen checks that an intact record with the
// wrong index — which no crash can produce — is corruption, even at the tail.
func TestOutOfSequenceRecordRefusesToOpen(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeRecords(t, filepath.Join(dir, "00000000000000000001.log"), 1, 2, 4)

	_, err := storage.OpenLog(context.Background(), logConfig(dir), clock.Real(), discardLogger())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of sequence")
}

// TestSegmentGapRefusesToOpen checks that a missing range between segments
// is corruption.
func TestSegmentGapRefusesToOpen(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeRecords(t, filepath.Join(dir, "00000000000000000001.log"), 1, 2, 3)
	writeRecords(t, filepath.Join(dir, "00000000000000000005.log"), 5, 6)

	_, err := storage.OpenLog(context.Background(), logConfig(dir), clock.Real(), discardLogger())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 4")
}

// TestEmptyTrailingSegmentIsAccepted checks the state a crash right after
// starting a new segment leaves: an empty last segment, which is valid.
func TestEmptyTrailingSegmentIsAccepted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeRecords(t, filepath.Join(dir, "00000000000000000001.log"), 1, 2, 3)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "00000000000000000004.log"), nil, 0o600))

	l := openLog(t, logConfig(dir), clock.Real())
	defer func() { require.NoError(t, l.Close()) }()
	assert.Equal(t, uint64(3), l.LastIndex())

	appendN(t, l, 2)
	assert.Equal(t, indexes(1, 5), replayAll(t, l, 1))
}

// TestUnexpectedFileRefusesToOpen checks that the log directory holds nothing
// but segments, so a misconfigured directory is caught.
func TestUnexpectedFileRefusesToOpen(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"notes.txt", "1.log", "0000000000000000000x.log"} {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))

		_, err := storage.OpenLog(context.Background(), logConfig(dir), clock.Real(), discardLogger())

		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "unexpected entry", name)
	}
}
