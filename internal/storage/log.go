package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/nightCode42/gokvx/internal/clock"
	"github.com/nightCode42/gokvx/internal/config"
	"github.com/nightCode42/gokvx/internal/kverr"
)

// LogConfig configures a Log.
type LogConfig struct {
	// Dir is the directory holding the segment files; created if missing.
	Dir string
	// SegmentBytes is the size at which a new segment is started
	// (storage.wal_segment_bytes). A record larger than this gets a segment
	// of its own.
	SegmentBytes int64
	// Fsync decides when appended records are flushed to disk (KV-STO-005).
	Fsync config.FsyncPolicy
	// FsyncInterval is the flush period when Fsync is interval.
	FsyncInterval time.Duration
}

// Validate checks that the configuration can open a log.
func (c LogConfig) Validate() error {
	var problems []kverr.FieldViolation
	if c.Dir == "" {
		problems = append(problems, kverr.FieldViolation{Field: "Dir", Description: "is required"})
	}
	if c.SegmentBytes <= 0 {
		problems = append(problems, kverr.FieldViolation{Field: "SegmentBytes", Description: "must be greater than zero"})
	}
	switch c.Fsync {
	case config.FsyncAlways, config.FsyncOS:
	case config.FsyncInterval:
		if c.FsyncInterval <= 0 {
			problems = append(problems, kverr.FieldViolation{Field: "FsyncInterval", Description: "must be greater than zero"})
		}
	default:
		problems = append(problems, kverr.FieldViolation{Field: "Fsync", Description: "must be always, interval, or os"})
	}
	if len(problems) > 0 {
		return kverr.New(kverr.ReasonInvalidArgument, "command log configuration is invalid").WithViolations(problems...)
	}
	return nil
}

// Log is the file-backed command log: the durable, ordered record of every
// command, from which the state machine is rebuilt after a crash (spec §9.1).
// Records are numbered from 1 without gaps. A Log is safe for concurrent use.
type Log struct {
	// cfg is the validated configuration.
	cfg LogConfig
	// logger receives warnings about repaired damage and background failures.
	logger *slog.Logger

	// mu serializes appends and guards every field below. File writes and
	// fsyncs happen under it: the log's order is its correctness.
	mu sync.Mutex
	// segments lists the segment files in index order; the last is active.
	segments []segment
	// active is the open file of the last segment.
	active *os.File
	// activeSize is the size of the active segment in bytes.
	activeSize int64
	// lastIndex is the index of the last durable-or-written record; 0 when
	// the log is empty.
	lastIndex uint64
	// dirty reports whether records were written since the last fsync.
	dirty bool
	// failed holds the first write or fsync failure. After one, the file's
	// contents are uncertain, so every later append is refused.
	failed error
	// closed is set by Close.
	closed bool

	// stop, closed by Close, ends the background syncer; done is closed when
	// it has exited. Both are nil unless the policy is interval.
	stop, done chan struct{}
	// onSync, when set by tests, is called after every fsync of the active
	// segment, with mu held.
	onSync func()
}

// OpenLog opens the command log in cfg.Dir, creating it if needed. Opening
// verifies every record: a damaged tail is truncated with a warning, and
// damage followed by intact records makes OpenLog fail (KV-STO-004). With the
// interval policy, clk drives the background fsync.
func OpenLog(ctx context.Context, cfg LogConfig, clk clock.Clock, logger *slog.Logger) (*Log, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, kverr.Wrap(kverr.ReasonInternal, err, "command log directory could not be created")
	}

	l := &Log{cfg: cfg, logger: logger}
	if err := l.recover(); err != nil {
		return nil, err
	}
	if cfg.Fsync == config.FsyncInterval {
		l.stop, l.done = make(chan struct{}), make(chan struct{})
		go l.runSyncer(clk.NewTicker(cfg.FsyncInterval))
	}
	return l, nil
}

// Append writes command as the next record and returns its index. With the
// always policy, the record is on disk before Append returns (KV-STO-005).
// After a write or fsync failure, the log refuses further appends.
func (l *Log) Append(ctx context.Context, command []byte) (uint64, error) {
	if err := contextError(ctx); err != nil {
		return 0, err
	}
	if len(command) > maxCommandSize {
		return 0, kverr.Newf(kverr.ReasonInvalidArgument, "command of %d bytes exceeds the %d-byte record limit",
			len(command), maxCommandSize)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.writable(); err != nil {
		return 0, err
	}

	index := l.lastIndex + 1
	record := encodeRecord(index, command)
	if l.activeSize > 0 && l.activeSize+int64(len(record)) > l.cfg.SegmentBytes {
		if err := l.rotate(index); err != nil {
			l.failed = err
			return 0, err
		}
	}
	if _, err := l.active.Write(record); err != nil {
		l.failed = kverr.Wrap(kverr.ReasonInternal, err, "command log write failed")
		return 0, l.failed
	}
	l.activeSize += int64(len(record))
	l.dirty = true
	if l.cfg.Fsync == config.FsyncAlways {
		if err := l.sync(); err != nil {
			return 0, err
		}
	}

	l.lastIndex = index
	l.segments[len(l.segments)-1].last = index
	return index, nil
}

// LastIndex returns the index of the last record, or 0 for an empty log.
func (l *Log) LastIndex() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastIndex
}

// Replay calls fn with the index and command of every record from index
// from onward, in order, up to the last record present when Replay was
// called. The command is valid only during the call. Replay stops at the
// first error from fn or ctx.
func (l *Log) Replay(ctx context.Context, from uint64, fn func(index uint64, command []byte) error) error {
	l.mu.Lock()
	segs := slices.Clone(l.segments)
	last := l.lastIndex
	l.mu.Unlock()

	for _, seg := range segs {
		if seg.last < from || seg.first > last {
			continue
		}
		if err := replaySegment(ctx, seg, from, last, fn); err != nil {
			return err
		}
	}
	return nil
}

// replaySegment calls fn for the records of seg with index in [from, last].
func replaySegment(ctx context.Context, seg segment, from, last uint64, fn func(uint64, []byte) error) error {
	data, err := os.ReadFile(seg.path)
	if err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "command log segment could not be read")
	}
	for offset, want := 0, seg.first; want <= min(seg.last, last); want++ {
		if err := contextError(ctx); err != nil {
			return err
		}
		index, command, size, err := decodeRecord(data[offset:])
		if err == nil && index != want {
			err = fmt.Errorf("record index %d, expected %d", index, want)
		}
		if err != nil {
			return kverr.Wrap(kverr.ReasonInternal, err,
				fmt.Sprintf("command log segment %s changed on disk at offset %d", filepath.Base(seg.path), offset))
		}
		if index >= from {
			if err := fn(index, command); err != nil {
				return fmt.Errorf("storage.Log.Replay: %w", err)
			}
		}
		offset += size
	}
	return nil
}

// Close flushes and closes the log. Closing twice is harmless.
func (l *Log) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	l.mu.Unlock()

	// Wait for the syncer outside the lock, which it may be waiting for.
	if l.stop != nil {
		close(l.stop)
		<-l.done
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	var syncErr error
	if l.failed == nil && l.dirty {
		syncErr = l.sync()
	}
	if err := l.active.Close(); err != nil {
		return kverr.Wrap(kverr.ReasonInternal, errors.Join(syncErr, err), "command log could not be closed")
	}
	return syncErr
}

// writable returns an error if the log cannot accept records. The caller
// holds l.mu.
func (l *Log) writable() error {
	if l.closed {
		return kverr.Wrap(kverr.ReasonInternal, errClosed, "command log is closed")
	}
	if l.failed != nil {
		return kverr.Wrap(kverr.ReasonInternal, l.failed, "command log is unusable after an earlier failure")
	}
	return nil
}

// sync flushes the active segment. A failure is sticky: the kernel may have
// dropped the unflushed data, so the log stops accepting writes. The caller
// holds l.mu.
func (l *Log) sync() error {
	if err := l.active.Sync(); err != nil {
		l.failed = kverr.Wrap(kverr.ReasonInternal, err, "command log fsync failed")
		return l.failed
	}
	l.dirty = false
	if l.onSync != nil {
		l.onSync()
	}
	return nil
}

// rotate flushes and closes the active segment and starts a new one whose
// first record will be next. The caller holds l.mu.
func (l *Log) rotate(next uint64) error {
	if l.dirty {
		if err := l.sync(); err != nil {
			return err
		}
	}
	if err := l.active.Close(); err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "command log segment could not be closed")
	}
	path := filepath.Join(l.cfg.Dir, segmentName(next))
	f, err := createSegment(path, l.cfg.Dir)
	if err != nil {
		return err
	}
	l.segments = append(l.segments, segment{first: next, last: next - 1, path: path})
	l.active, l.activeSize = f, 0
	return nil
}

// createSegment creates a new, empty segment file and makes its directory
// entry durable, so the file itself survives a crash.
func createSegment(path, dir string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // path is built from the configured directory.
	if err != nil {
		return nil, kverr.Wrap(kverr.ReasonInternal, err, "command log segment could not be created")
	}
	if err := syncDir(dir); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// runSyncer flushes the log on every tick until stop is closed, for the
// interval policy. It owns ticker.
func (l *Log) runSyncer(ticker clock.Ticker) {
	defer close(l.done)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C():
			l.syncIfDirty()
		case <-l.stop:
			return
		}
	}
}

// syncIfDirty flushes the active segment if records were written since the
// last flush. A failure is logged here, because no caller is waiting for it;
// the next append reports it too.
func (l *Log) syncIfDirty() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.dirty || l.failed != nil || l.closed {
		return
	}
	if err := l.sync(); err != nil {
		l.logger.Error("command log background fsync failed; refusing further writes", slog.Any("error", err))
	}
}
