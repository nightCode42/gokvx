package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// A segment file is named after the index of its first record, zero-padded
// to 20 digits so that names sort in index order: 00000000000000000001.log.
// The naming is part of the on-disk format.
const (
	// segmentSuffix ends every segment file name.
	segmentSuffix = ".log"
	// segmentDigits is the width of the index in a segment file name.
	segmentDigits = 20
)

// segment describes one segment file of the log.
type segment struct {
	// first is the index of the segment's first record.
	first uint64
	// last is the index of its last record; first-1 while it is empty.
	last uint64
	// path is the segment file's path.
	path string
}

// segmentName returns the file name of the segment starting at first.
func segmentName(first uint64) string {
	return fmt.Sprintf("%0*d%s", segmentDigits, first, segmentSuffix)
}

// parseSegmentName returns the first index encoded in a segment file name,
// and false for any name that is not exactly a segment name.
func parseSegmentName(name string) (uint64, bool) {
	digits, ok := strings.CutSuffix(name, segmentSuffix)
	if !ok || len(digits) != segmentDigits {
		return 0, false
	}
	first, err := strconv.ParseUint(digits, 10, 64)
	if err != nil || segmentName(first) != name {
		return 0, false
	}
	return first, true
}

// listSegments returns the segments in dir in index order, with last not yet
// known. Any other entry in dir is an error: the log directory holds nothing
// but segments, and an unexpected file suggests the wrong directory.
func listSegments(dir string) ([]segment, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, kverr.Wrap(kverr.ReasonInternal, err, "command log directory could not be read")
	}
	var segs []segment
	for _, e := range entries {
		first, ok := parseSegmentName(e.Name())
		if !ok || !e.Type().IsRegular() {
			return nil, kverr.Newf(kverr.ReasonInternal,
				"command log directory %s contains an unexpected entry %q", dir, e.Name())
		}
		segs = append(segs, segment{first: first, path: filepath.Join(dir, e.Name())})
	}
	slices.SortFunc(segs, func(a, b segment) int {
		switch {
		case a.first < b.first:
			return -1
		case a.first > b.first:
			return 1
		default:
			return 0
		}
	})
	return segs, nil
}
