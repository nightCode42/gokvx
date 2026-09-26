package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// FormatVersion is the version of the on-disk format this binary reads and
// writes. It is recorded in every data directory and changes only with an
// incompatible format change (KV-STO-009).
const FormatVersion = 1

// The layout of a data directory, part of the on-disk format
// (docs/engineering/storage-format.md).
const (
	// versionFile names the marker holding the directory's format version.
	// It contains the version in decimal followed by a newline: "1\n".
	versionFile = "storage_version"
	// logDir names the directory holding the command log's segments.
	logDir = "log"
	// engineDir names the directory holding the Pebble database.
	engineDir = "engine"
)

// LogDir returns the directory of the command log within dataDir.
func LogDir(dataDir string) string {
	return filepath.Join(dataDir, logDir)
}

// EngineDir returns the directory of the storage engine within dataDir.
func EngineDir(dataDir string) string {
	return filepath.Join(dataDir, engineDir)
}

// PrepareDataDir makes dir ready for this binary before anything else in it
// is read (KV-STO-009). A new or empty directory receives a version marker;
// an existing one must carry a marker naming FormatVersion. A directory with
// data but no marker is refused, since its contents are not known to be
// gokvx data.
func PrepareDataDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "data directory could not be created")
	}

	content, err := os.ReadFile(filepath.Join(dir, versionFile)) //nolint:gosec // dir is the configured data directory.
	switch {
	case err == nil:
		return checkVersion(dir, string(content))
	case errors.Is(err, fs.ErrNotExist):
		return initializeDataDir(dir)
	default:
		return kverr.Wrap(kverr.ReasonInternal, err, "storage_version marker could not be read")
	}
}

// checkVersion accepts a marker naming FormatVersion and refuses anything
// else with a message an operator can act on.
func checkVersion(dir, content string) error {
	version, err := strconv.Atoi(strings.TrimSpace(content))
	if err != nil {
		return kverr.Newf(kverr.ReasonInternal,
			"data directory %s has an unreadable storage_version marker %q", dir, content)
	}
	if version != FormatVersion {
		return kverr.Newf(kverr.ReasonInternal,
			"data directory %s uses storage format %d, but this binary supports only format %d",
			dir, version, FormatVersion)
	}
	return nil
}

// initializeDataDir writes the version marker into a directory that has none,
// provided the directory holds no data. A leftover temporary marker from an
// interrupted earlier attempt is not data.
func initializeDataDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "data directory could not be read")
	}
	for _, e := range entries {
		if e.Name() != versionFile+".tmp" {
			return kverr.Newf(kverr.ReasonInternal,
				"data directory %s contains data but no storage_version marker; refusing to use it", dir)
		}
	}
	return writeFileAtomic(dir, versionFile, []byte(strconv.Itoa(FormatVersion)+"\n"))
}

// writeFileAtomic creates dir/name with content so that, after a crash,
// either the complete file exists or none does: it writes and flushes a
// temporary file, renames it into place, and flushes the directory.
func writeFileAtomic(dir, name string, content []byte) error {
	path := filepath.Join(dir, name)
	tmp := path + ".tmp"

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //nolint:gosec // dir is the configured data directory.
	if err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, name+" could not be created")
	}
	_, writeErr := f.Write(content)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, name+" could not be written")
	}
	if err := os.Rename(tmp, path); err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, name+" could not be put in place")
	}
	return syncDir(dir)
}
