package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// samplePath is the configuration used by the local Docker Compose stack.
const samplePath = "../../deploy/compose/gokvx.yaml"

// result is the outcome of one in-process run of the binary.
type result struct {
	code   int
	stdout string
	stderr string
}

// execute runs the command line with the given environment and captures its
// output and exit code.
func execute(environ []string, args ...string) result {
	var stdout, stderr bytes.Buffer
	code := run(args, environ, &stdout, &stderr)
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// writeConfig writes a configuration file in a temporary directory.
func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "gokvx.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// TestSampleConfigIsValid_KV_CFG_003 checks that the Compose configuration
// passes `gokvx config validate`.
// Verifies: KV-CFG-003.
func TestSampleConfigIsValid_KV_CFG_003(t *testing.T) {
	t.Parallel()

	r := execute(nil, "config", "validate", "--config", samplePath)

	assert.Equal(t, exitOK, r.code, r.stderr)
	assert.Equal(t, "configuration is valid\n", r.stdout)
	assert.Empty(t, r.stderr)
}

// TestValidateReportsEveryViolation_KV_CFG_002 checks that an invalid file
// fails with a non-zero exit and lists each offending key.
// Verifies: KV-CFG-002, KV-CFG-003.
func TestValidateReportsEveryViolation_KV_CFG_002(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, "storage:\n  fsync: never\ncluster:\n  raft:\n    pre_vote: false\n")

	r := execute(nil, "config", "validate", "--config", path)

	assert.Equal(t, exitFailure, r.code)
	assert.Empty(t, r.stdout)
	assert.Contains(t, r.stderr, "error: configuration is invalid\n")
	for _, key := range []string{"auth.jwt.issuer_url", "cluster.raft.pre_vote", "storage.fsync", "tls.allowed_client_sans"} {
		assert.Contains(t, r.stderr, "  - "+key+": ")
	}
}

// TestValidateRejectsUnknownKey_KV_CFG_020 checks that a typo in the file is
// reported by name.
// Verifies: KV-CFG-020.
func TestValidateRejectsUnknownKey_KV_CFG_020(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, "storage:\n  fsinc: always\n")

	r := execute(nil, "config", "validate", "--config", path)

	assert.Equal(t, exitFailure, r.code)
	assert.Contains(t, r.stderr, "  - storage.fsinc: is not a configuration key\n")
}

// TestValidateAppliesEnvironmentAndFlags_KV_CFG_001 checks that the command
// validates the effective configuration, layering environment variables and
// flags over the file exactly as a starting node would.
// Verifies: KV-CFG-001, KV-CFG-003.
func TestValidateAppliesEnvironmentAndFlags_KV_CFG_001(t *testing.T) {
	t.Parallel()

	environ := []string{"GOKVX_STORAGE_FSYNC=never"}

	fromEnv := execute(environ, "config", "validate", "--config", samplePath)
	assert.Equal(t, exitFailure, fromEnv.code)
	assert.Contains(t, fromEnv.stderr, "  - storage.fsync: ")

	flagWins := execute(environ, "config", "validate", "--config", samplePath, "--storage.fsync=os")
	assert.Equal(t, exitOK, flagWins.code, flagWins.stderr)
}

// TestValidateWarnsAboutUnsafeSettings_KV_CFG_021 checks that a valid but
// unsafe configuration passes with one warning per unsafe setting.
// Verifies: KV-CFG-021.
func TestValidateWarnsAboutUnsafeSettings_KV_CFG_021(t *testing.T) {
	t.Parallel()

	r := execute(nil, "config", "validate", "--config", samplePath, "--storage.fsync=os")

	assert.Equal(t, exitOK, r.code, r.stderr)
	assert.Equal(t,
		"configuration is valid\n"+
			"warning: storage.fsync=os: a crash can lose any acknowledged write the operating system has not flushed\n",
		r.stdout)
}

// TestValidatePrintRedactsCredentials_KV_CFG_006 checks that the printed
// configuration never shows credentials embedded in URLs.
// Verifies: KV-CFG-006.
func TestValidatePrintRedactsCredentials_KV_CFG_006(t *testing.T) {
	t.Parallel()

	r := execute(nil, "config", "validate", "--config", samplePath, "--print",
		"--auth.jwt.jwks_url=https://client:s3cret@stub-issuer:8443/jwks.json")

	require.Equal(t, exitOK, r.code, r.stderr)
	assert.Contains(t, r.stdout, "jwks_url: https://REDACTED@stub-issuer:8443/jwks.json\n")
	assert.NotContains(t, r.stdout, "s3cret")
	assert.Contains(t, r.stdout, "environment: development\n")
}

// TestValidateMissingFile checks that an unreadable file is reported with its
// cause.
func TestValidateMissingFile(t *testing.T) {
	t.Parallel()

	r := execute(nil, "config", "validate", "--config", filepath.Join(t.TempDir(), "missing.yaml"))

	assert.Equal(t, exitFailure, r.code)
	assert.Contains(t, r.stderr, "error: configuration file could not be read: ")
	assert.Contains(t, r.stderr, "missing.yaml")
}

// TestVersion_KV_CFG_012 checks the build report.
// Verifies: KV-CFG-012.
func TestVersion_KV_CFG_012(t *testing.T) {
	t.Parallel()

	r := execute(nil, "version")

	assert.Equal(t, exitOK, r.code)
	assert.Equal(t,
		"gokvx "+Version+"\ncommit: "+Commit+"\nbuilt:  "+BuildDate+"\ngo:     "+runtime.Version()+"\n",
		r.stdout)
}

// TestUsageMistakes checks that unknown commands, arguments, and flags fail
// with a message instead of being ignored.
func TestUsageMistakes(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"unknown command": {"serve-forever"},
		"extra argument":  {"version", "now"},
		"unknown flag":    {"config", "validate", "--storage.fsinc=os"},
	}

	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := execute(nil, args...)

			assert.Equal(t, exitFailure, r.code)
			assert.Contains(t, r.stderr, "error: ")
		})
	}
}

// TestNoArgumentsShowsHelp checks that running the binary bare explains how to
// use it.
func TestNoArgumentsShowsHelp(t *testing.T) {
	t.Parallel()

	r := execute(nil)

	assert.Equal(t, exitOK, r.code)
	assert.Contains(t, r.stdout, "Usage:")
	assert.Contains(t, r.stdout, "config")
	assert.Contains(t, r.stdout, "version")
}
