package storage_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/clock"
	"github.com/nightCode42/gokvx/internal/config"
	"github.com/nightCode42/gokvx/internal/storage"
)

// crashChildEnv, when set to a directory, makes the test binary run the crash
// workload in that directory instead of the tests (see TestMain).
const crashChildEnv = "GOKVX_STORAGE_CRASH_CHILD"

// crashSegmentBytes keeps segments small in the crash workload, so kills
// also land while a segment is being rotated.
const crashSegmentBytes = 1024

// runCrashChild is the crash workload: it appends records with the always
// policy forever, printing "ack <index>" after each Append returns — that is,
// after the record is durable. It never returns normally; the parent kills
// it, so it returns only an error.
func runCrashChild(dir string) error {
	cfg := storage.LogConfig{Dir: dir, SegmentBytes: crashSegmentBytes, Fsync: config.FsyncAlways}
	l, err := storage.OpenLog(context.Background(), cfg, clock.Real(), discardLogger())
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	for {
		index, err := l.Append(context.Background(), command(l.LastIndex()+1))
		if err != nil {
			return fmt.Errorf("append: %w", err)
		}
		fmt.Printf("ack %d\n", index)
	}
}

// TestCrashRecovery_KV_STO_020 kills a process appending to the log at random
// moments, several times over the same directory, and checks after each kill
// that every acknowledged record is present and intact, and that no record is
// partial. The seed is logged so a failure can be reproduced.
// Verifies: KV-STO-020, KV-STO-003, KV-STO-004, KV-STO-005.
func TestCrashRecovery_KV_STO_020(t *testing.T) {
	t.Parallel()

	seed := uint64(time.Now().UnixNano()) //nolint:gosec // a seed only needs to vary between runs.
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewPCG(seed, seed)) //nolint:gosec // a reproducible sequence, not a secret.
	dir := t.TempDir()

	for round := range 5 {
		acked := appendUntilKilled(t, dir, 1+rng.IntN(150))

		cfg := storage.LogConfig{Dir: dir, SegmentBytes: crashSegmentBytes, Fsync: config.FsyncAlways}
		l := openLog(t, cfg, clock.Real())
		last := l.LastIndex()
		assert.GreaterOrEqual(t, last, acked, "round %d: an acknowledged record was lost", round)
		assert.Equal(t, indexes(1, last), replayAll(t, l, 1), "round %d", round)
		require.NoError(t, l.Close())
	}
}

// appendUntilKilled runs the crash workload on dir, kills it after it has
// acknowledged killAfter records, and returns the highest acknowledged index.
// The kill arrives while the child keeps appending, so it lands at an
// arbitrary point: mid-write, mid-fsync, or mid-rotation.
func appendUntilKilled(t *testing.T, dir string, killAfter int) uint64 {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$") //nolint:gosec // re-runs this test binary as the child.
	cmd.Env = append(os.Environ(), crashChildEnv+"="+dir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	var acks int
	var lastAck uint64
	lines := bufio.NewScanner(stdout)
	for lines.Scan() {
		var index uint64
		if _, err := fmt.Sscanf(lines.Text(), "ack %d", &index); err != nil {
			continue
		}
		lastAck, acks = index, acks+1
		if acks == killAfter {
			require.NoError(t, cmd.Process.Kill())
		}
	}
	require.Error(t, cmd.Wait(), "the child must be killed, not exit")
	require.GreaterOrEqual(t, acks, killAfter, "the child stopped early: %s", stderr.String())
	return lastAck
}
