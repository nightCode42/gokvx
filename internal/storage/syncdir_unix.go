//go:build !windows

package storage

import (
	"os"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// syncDir makes the entries of dir durable: a file created or renamed in dir
// survives a crash only once dir itself is fsynced.
func syncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // dir is the node's configured data directory.
	if err != nil {
		return kverr.Wrap(kverr.ReasonInternal, err, "directory could not be opened for sync")
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil {
		return kverr.Wrap(kverr.ReasonInternal, syncErr, "directory could not be synced")
	}
	if closeErr != nil {
		return kverr.Wrap(kverr.ReasonInternal, closeErr, "directory could not be closed")
	}
	return nil
}
