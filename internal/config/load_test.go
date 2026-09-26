package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/config"
	"github.com/nightCode42/gokvx/internal/kverr"
)

// writeFile writes content to a YAML file in a temporary directory and
// returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "gokvx.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// parseFlags registers the configuration flags on a new flag set and parses
// args.
func parseFlags(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()

	fs := pflag.NewFlagSet("gokvx", pflag.ContinueOnError)
	require.NoError(t, config.RegisterFlags(fs))
	require.NoError(t, fs.Parse(args))
	return fs
}

// violationFields returns the keys named by the violations in err.
func violationFields(t *testing.T, err error) []string {
	t.Helper()

	var kerr *kverr.Error
	require.ErrorAs(t, err, &kerr)
	require.Equal(t, kverr.ReasonInvalidArgument, kerr.Reason())

	fields := make([]string, 0, len(kerr.Violations()))
	for _, v := range kerr.Violations() {
		fields = append(fields, v.Field)
	}
	return fields
}

// TestLoadWithoutSourcesReturnsDefaults checks that the defaults are the
// bottom layer.
func TestLoadWithoutSourcesReturnsDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(config.Options{})

	require.NoError(t, err)
	assert.Equal(t, config.Defaults(), cfg)
}

// TestLoadPrecedence_KV_CFG_001 checks that each layer overrides the ones
// before it: flags over environment over file over defaults.
// Verifies: KV-CFG-001.
func TestLoadPrecedence_KV_CFG_001(t *testing.T) {
	t.Parallel()

	path := writeFile(t, `
node:
  id: from-file
storage:
  fsync_interval: 1s
  snapshot_entries: 111
  wal_retain_segments: 9
limits:
  max_key_bytes: 111
`)
	environ := []string{
		"GOKVX_NODE_ID=from-env",
		"GOKVX_STORAGE_SNAPSHOT_ENTRIES=222",
		"GOKVX_LIMITS_MAX_KEY_BYTES=222",
		"PATH=/usr/bin",
	}
	flags := parseFlags(t, "--node.id=from-flag", "--limits.max_key_bytes=333")

	cfg, err := config.Load(config.Options{File: path, Environ: environ, Flags: flags})
	require.NoError(t, err)

	assert.Equal(t, "from-flag", cfg.Node.ID, "flag beats environment and file")
	assert.Equal(t, 333, cfg.Limits.MaxKeyBytes, "flag beats environment and file")
	assert.Equal(t, 222, cfg.Storage.SnapshotEntries, "environment beats file")
	assert.Equal(t, time.Second, cfg.Storage.FsyncInterval, "file beats defaults")
	assert.Equal(t, 9, cfg.Storage.WALRetainSegments, "an unset flag does not override the file")
	assert.Equal(t, config.Defaults().Storage.QuotaBytes, cfg.Storage.QuotaBytes, "untouched keys keep their default")
}

// TestLoadTypesFromEveryLayer checks that each value type is converted
// correctly from text in the environment and on the command line.
func TestLoadTypesFromEveryLayer(t *testing.T) {
	t.Parallel()

	environ := []string{
		"GOKVX_AUTH_JWT_AUDIENCE=gokvx, gokvx-admin ,",
		"GOKVX_AUTH_JWT_CLOCK_SKEW=30s",
		"GOKVX_TLS_RELOAD=false",
		"GOKVX_OBSERVABILITY_TRACING_SAMPLE_RATIO=0.5",
		"GOKVX_CLUSTER_SLOT_COUNT=1024",
		"GOKVX_STORAGE_FSYNC=interval",
	}
	flags := parseFlags(t, "--auth.jwt.algorithms=RS256,ES256", "--storage.quota_bytes=1073741824")

	cfg, err := config.Load(config.Options{Environ: environ, Flags: flags})
	require.NoError(t, err)

	assert.Equal(t, []string{"gokvx", "gokvx-admin"}, cfg.Auth.JWT.Audience)
	assert.Equal(t, 30*time.Second, cfg.Auth.JWT.ClockSkew)
	assert.False(t, cfg.TLS.Reload)
	assert.InDelta(t, 0.5, cfg.Observability.Tracing.SampleRatio, 1e-9)
	assert.Equal(t, uint32(1024), cfg.Cluster.SlotCount)
	assert.Equal(t, config.FsyncInterval, cfg.Storage.Fsync)
	assert.Equal(t, []string{"RS256", "ES256"}, cfg.Auth.JWT.Algorithms)
	assert.Equal(t, int64(1<<30), cfg.Storage.QuotaBytes)
}

// TestLoadRejectsUnknownFileKeys_KV_CFG_020 checks that a misspelled or
// invented key is reported by name instead of being ignored.
// Verifies: KV-CFG-020.
func TestLoadRejectsUnknownFileKeys_KV_CFG_020(t *testing.T) {
	t.Parallel()

	path := writeFile(t, `
storage:
  fsinc: always
auth:
  mode: enabled
  jwt:
    isuer_url: https://issuer.test/
invented: true
`)

	_, err := config.Load(config.Options{File: path})

	assert.Equal(t, []string{"auth.jwt.isuer_url", "invented", "storage.fsinc"}, violationFields(t, err))
}

// TestLoadRejectsUnknownEnvironmentVariables_KV_CFG_020 checks that a
// misspelled GOKVX_ variable is an error, while other variables and the ones
// Kubernetes injects for Services are ignored.
// Verifies: KV-CFG-020.
func TestLoadRejectsUnknownEnvironmentVariables_KV_CFG_020(t *testing.T) {
	t.Parallel()

	environ := []string{
		"GOKVX_STORAGE_FSINC=os",
		"GOKVX_CLIENT_SERVICE_HOST=10.0.0.1",
		"GOKVX_CLIENT_SERVICE_PORT=2379",
		"GOKVX_CLIENT_SERVICE_PORT_GRPC=2379",
		"GOKVX_CLIENT_PORT=tcp://10.0.0.1:2379",
		"GOKVX_CLIENT_PORT_2379_TCP=tcp://10.0.0.1:2379",
		"GOKVX_CLIENT_PORT_2379_TCP_ADDR=10.0.0.1",
		"HOME=/root",
	}

	_, err := config.Load(config.Options{Environ: environ})

	assert.Equal(t, []string{"GOKVX_STORAGE_FSINC"}, violationFields(t, err))
}

// TestLoadRejectsValueWhereSectionExpected checks that a section written as a
// scalar is reported rather than silently dropping its keys.
func TestLoadRejectsValueWhereSectionExpected(t *testing.T) {
	t.Parallel()

	path := writeFile(t, "storage: fast\n")

	_, err := config.Load(config.Options{File: path})

	assert.Equal(t, []string{"storage"}, violationFields(t, err))
}

// TestLoadEmptySectionKeepsDefaults checks that a section left empty, as a
// bare "storage:" line, does not erase the defaults beneath it.
func TestLoadEmptySectionKeepsDefaults(t *testing.T) {
	t.Parallel()

	path := writeFile(t, "storage:\nlimits: {}\nnode:\n  id: n1\n")

	cfg, err := config.Load(config.Options{File: path})
	require.NoError(t, err)

	assert.Equal(t, config.Defaults().Storage, cfg.Storage)
	assert.Equal(t, config.Defaults().Limits, cfg.Limits)
	assert.Equal(t, "n1", cfg.Node.ID)
}

// TestLoadReplacesListsWholesale checks that a list from a source replaces
// the default list rather than merging with it, including an empty list.
func TestLoadReplacesListsWholesale(t *testing.T) {
	t.Parallel()

	path := writeFile(t, "auth:\n  jwt:\n    audience: []\n    algorithms: [ES256]\n")

	cfg, err := config.Load(config.Options{File: path})
	require.NoError(t, err)

	assert.Empty(t, cfg.Auth.JWT.Audience)
	assert.Equal(t, []string{"ES256"}, cfg.Auth.JWT.Algorithms)
}

// TestLoadRejectsWrongType_KV_CFG_002 checks that a value that cannot be
// converted to its key's type fails with INVALID_ARGUMENT naming the key.
// Verifies: KV-CFG-002.
func TestLoadRejectsWrongType_KV_CFG_002(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		file    string
		environ []string
		key     string
	}{
		{"integer in file", "limits:\n  max_key_bytes: lots\n", nil, "max_key_bytes"},
		{"duration in file", "node:\n  shutdown_grace: soon\n", nil, "shutdown_grace"},
		{"boolean in environment", "", []string{"GOKVX_TLS_ENABLED=maybe"}, "enabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := config.Options{Environ: tt.environ}
			if tt.file != "" {
				opts.File = writeFile(t, tt.file)
			}

			_, err := config.Load(opts)

			require.Error(t, err)
			assert.Equal(t, kverr.ReasonInvalidArgument, kverr.ReasonOf(err))
			assert.Contains(t, err.Error(), tt.key)
		})
	}
}

// TestLoadMissingFile checks that an unreadable file is an INVALID_ARGUMENT
// error that keeps the operating system's reason as its cause.
func TestLoadMissingFile(t *testing.T) {
	t.Parallel()

	_, err := config.Load(config.Options{File: filepath.Join(t.TempDir(), "missing.yaml")})

	assert.Equal(t, kverr.ReasonInvalidArgument, kverr.ReasonOf(err))
	assert.True(t, errors.Is(err, os.ErrNotExist), "the cause is kept: %v", err)
}

// TestLoadRejectsMalformedYAML checks that a file that is not valid YAML is
// reported rather than treated as empty.
func TestLoadRejectsMalformedYAML(t *testing.T) {
	t.Parallel()

	_, err := config.Load(config.Options{File: writeFile(t, "node: [unclosed\n")})

	assert.Equal(t, kverr.ReasonInvalidArgument, kverr.ReasonOf(err))
}
