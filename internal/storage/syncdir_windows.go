//go:build windows

package storage

// syncDir does nothing on Windows, which cannot fsync a directory handle;
// NTFS journals directory changes itself. Production nodes run on Linux,
// where the directory is fsynced (syncdir_unix.go). Windows is supported for
// development only.
func syncDir(string) error {
	return nil
}
