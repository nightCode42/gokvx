package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/storage"
)

// TestPrepareDataDirMarksNewDirectory_KV_STO_009 checks that a new directory
// receives a marker with the current format, and is accepted afterwards.
// Verifies: KV-STO-009.
func TestPrepareDataDirMarksNewDirectory_KV_STO_009(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "data")

	require.NoError(t, storage.PrepareDataDir(dir))
	marker, err := os.ReadFile(filepath.Join(dir, "storage_version")) //nolint:gosec // the marker in the test's temporary directory.
	require.NoError(t, err)
	assert.Equal(t, "1\n", string(marker))

	require.NoError(t, storage.PrepareDataDir(dir), "a marked directory is accepted")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left behind")
}

// TestPrepareDataDirRefusesOtherFormats_KV_STO_009 checks that a directory
// written in another format, or one that is not a gokvx directory, is refused
// with a message naming the problem.
// Verifies: KV-STO-009.
func TestPrepareDataDirRefusesOtherFormats_KV_STO_009(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		message string
	}{
		{"newer format", map[string]string{"storage_version": "2\n"}, "uses storage format 2"},
		{"unreadable marker", map[string]string{"storage_version": "one\n"}, "unreadable storage_version"},
		{"data without marker", map[string]string{"log/00000000000000000001.log": ""}, "no storage_version marker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for name, content := range tt.files {
				path := filepath.Join(dir, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
			}

			err := storage.PrepareDataDir(dir)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.message)
		})
	}
}

// TestDataDirLayout pins the names inside a data directory, which are part of
// the on-disk format.
func TestDataDirLayout(t *testing.T) {
	t.Parallel()

	assert.Equal(t, filepath.Join("data", "log"), storage.LogDir("data"))
	assert.Equal(t, filepath.Join("data", "engine"), storage.EngineDir("data"))
}

// TestPrepareDataDirRecoversInterruptedMarker checks that a temporary marker
// left by a crash during initialization does not make the directory look
// like foreign data.
func TestPrepareDataDirRecoversInterruptedMarker(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "storage_version.tmp"), []byte("1"), 0o600))

	require.NoError(t, storage.PrepareDataDir(dir))

	marker, err := os.ReadFile(filepath.Join(dir, "storage_version")) //nolint:gosec // the marker in the test's temporary directory.
	require.NoError(t, err)
	assert.Equal(t, "1\n", string(marker))
}
