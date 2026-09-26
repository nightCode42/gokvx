package storage

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// errOutOfSequence reports an intact record whose index is not the next one.
// A crash cannot produce it, so it is always treated as corruption.
var errOutOfSequence = errors.New("record index is out of sequence")

// recover loads the segments in the log directory, verifies every record,
// repairs a torn tail, and opens the last segment for appending
// (KV-STO-003, KV-STO-004). It runs once, from OpenLog.
func (l *Log) recover() error {
	segs, err := listSegments(l.cfg.Dir)
	if err != nil {
		return err
	}
	if len(segs) == 0 {
		return l.startFirstSegment()
	}

	for i := range segs {
		isLast := i == len(segs)-1
		if i > 0 && segs[i].first != segs[i-1].last+1 {
			return corruption(segs[i].path, 0, fmt.Errorf("segment starts at index %d, expected %d",
				segs[i].first, segs[i-1].last+1))
		}
		size, err := l.verifySegment(&segs[i], isLast)
		if err != nil {
			return err
		}
		if isLast {
			l.activeSize = size
		}
	}

	last := segs[len(segs)-1]
	f, err := os.OpenFile(last.path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "command log segment could not be opened")
	}
	l.segments, l.active, l.lastIndex = segs, f, last.last
	return nil
}

// startFirstSegment creates the first segment of a new, empty log.
func (l *Log) startFirstSegment() error {
	path := filepath.Join(l.cfg.Dir, segmentName(1))
	f, err := createSegment(path, l.cfg.Dir)
	if err != nil {
		return err
	}
	l.segments = []segment{{first: 1, last: 0, path: path}}
	l.active = f
	return nil
}

// verifySegment checks every record of seg, sets seg.last, and returns the
// segment's valid size in bytes. Damage in the last segment with no intact
// record after it is a torn tail: the segment is truncated before it and a
// warning is logged. Any other damage is corruption and fails.
func (l *Log) verifySegment(seg *segment, isLast bool) (int64, error) {
	data, err := os.ReadFile(seg.path)
	if err != nil {
		return 0, kverr.Wrap(kverr.ReasonInternal, err, "command log segment could not be read")
	}

	offset, next := 0, seg.first
	var damage error
	for offset < len(data) {
		index, _, size, err := decodeRecord(data[offset:])
		if err == nil && index != next {
			err = fmt.Errorf("%w: found %d, expected %d", errOutOfSequence, index, next)
		}
		if err != nil {
			damage = err
			break
		}
		offset += size
		next++
	}
	seg.last = next - 1

	if damage == nil {
		return int64(offset), nil
	}
	if !isLast || errors.Is(damage, errOutOfSequence) || hasLaterRecord(data[offset:], next) {
		return 0, corruption(seg.path, offset, damage)
	}
	return int64(offset), l.truncateTail(seg.path, offset, damage)
}

// truncateTail cuts a torn tail off the last segment and flushes the result
// (KV-STO-004).
func (l *Log) truncateTail(path string, offset int, damage error) error {
	if err := os.Truncate(path, int64(offset)); err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "torn command log tail could not be truncated")
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0o600) //nolint:gosec // path is built from the configured directory.
	if err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "command log segment could not be opened")
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "truncated command log segment could not be flushed")
	}

	l.logger.Warn("command log tail was damaged, probably by a crash during a write; truncated it",
		slog.String("segment", filepath.Base(path)),
		slog.Int("offset", offset),
		slog.String("damage", damage.Error()))
	return nil
}

// corruption returns the error for damage that is not a torn tail. The node
// must not start on it: records after the damage would be lost or misread.
func corruption(path string, offset int, damage error) error {
	return kverr.Wrap(kverr.ReasonInternal, damage, fmt.Sprintf(
		"command log is corrupted in segment %s at offset %d; refusing to open it, restore the node from a backup or a replica",
		filepath.Base(path), offset))
}
