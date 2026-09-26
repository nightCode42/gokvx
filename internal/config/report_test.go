package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/config"
)

// TestSafeConfigHasNoWarnings checks that the defaults weaken nothing.
func TestSafeConfigHasNoWarnings(t *testing.T) {
	t.Parallel()

	cfg := validConfig()

	assert.Empty(t, cfg.Warnings())
}

// TestEachUnsafeSettingWarnsOnce_KV_CFG_021 checks that every setting that
// weakens safety produces exactly one warning of its own, naming the setting.
// Verifies: KV-CFG-021.
func TestEachUnsafeSettingWarnsOnce_KV_CFG_021(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(c *config.Config)
		setting string
	}{
		{"auth disabled", func(c *config.Config) { c.Auth.Mode = config.AuthModeDisabled }, "auth.mode=disabled"},
		{"remote allowed", func(c *config.Config) { c.Auth.InsecureAllowRemote = true }, "auth.insecure_allow_remote=true"},
		{"tls off", func(c *config.Config) { c.TLS.Enabled = false }, "tls.enabled=false"},
		{"fsync interval", func(c *config.Config) { c.Storage.Fsync = config.FsyncInterval }, "storage.fsync=interval"},
		{"fsync os", func(c *config.Config) { c.Storage.Fsync = config.FsyncOS }, "storage.fsync=os"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validConfig()
			tt.mutate(&cfg)

			warnings := cfg.Warnings()
			require.Len(t, warnings, 1)
			assert.Equal(t, tt.setting, warnings[0].Setting)
			assert.NotEmpty(t, warnings[0].Consequence)
			assert.Equal(t, tt.setting+": "+warnings[0].Consequence, warnings[0].String())
		})
	}
}

// TestUnsafeSettingsWarnDistinctly_KV_CFG_021 checks that several unsafe
// settings together produce one distinct warning each.
// Verifies: KV-CFG-021.
func TestUnsafeSettingsWarnDistinctly_KV_CFG_021(t *testing.T) {
	t.Parallel()

	cfg := devWithoutAuth()
	cfg.Auth.InsecureAllowRemote = true
	cfg.Storage.Fsync = config.FsyncOS

	settings := make([]string, 0, 4)
	for _, w := range cfg.Warnings() {
		settings = append(settings, w.Setting)
	}

	assert.Equal(t, []string{
		"auth.mode=disabled",
		"auth.insecure_allow_remote=true",
		"tls.enabled=false",
		"storage.fsync=os",
	}, settings)
}

// TestRedactedRemovesURLCredentials_KV_CFG_006 checks that credentials
// embedded in URLs never reach a log, and that nothing else changes.
// Verifies: KV-CFG-006.
func TestRedactedRemovesURLCredentials_KV_CFG_006(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Auth.JWT.IssuerURL = "https://client:s3cret@issuer.test/"
	cfg.Auth.JWT.JWKSURL = "https://token@issuer.test/jwks.json"
	cfg.Observability.Tracing.OTLPEndpoint = "https://user:pw@collector.test:4318"

	redacted := cfg.Redacted()

	assert.Equal(t, "https://REDACTED@issuer.test/", redacted.Auth.JWT.IssuerURL)
	assert.Equal(t, "https://REDACTED@issuer.test/jwks.json", redacted.Auth.JWT.JWKSURL)
	assert.Equal(t, "https://REDACTED@collector.test:4318", redacted.Observability.Tracing.OTLPEndpoint)

	out, err := redacted.YAML()
	require.NoError(t, err)
	assert.NotContains(t, string(out), "s3cret")
	assert.NotContains(t, string(out), "token@")

	assert.Equal(t, "https://client:s3cret@issuer.test/", cfg.Auth.JWT.IssuerURL, "the original is unchanged")
	redacted.Auth.JWT.IssuerURL = cfg.Auth.JWT.IssuerURL
	redacted.Auth.JWT.JWKSURL = cfg.Auth.JWT.JWKSURL
	redacted.Observability.Tracing.OTLPEndpoint = cfg.Observability.Tracing.OTLPEndpoint
	assert.Equal(t, cfg, redacted, "nothing else changes")
}

// TestRedactedLeavesPlainAddressesAlone checks values without credentials,
// including a host:port that is not a URL.
func TestRedactedLeavesPlainAddressesAlone(t *testing.T) {
	t.Parallel()

	cfg := validConfig()

	redacted := cfg.Redacted()

	assert.Equal(t, cfg.Observability.Tracing.OTLPEndpoint, redacted.Observability.Tracing.OTLPEndpoint)
	assert.Equal(t, cfg.Auth.JWT.IssuerURL, redacted.Auth.JWT.IssuerURL)
}

// TestRedactedSharesNoLists checks that changing a list in the redacted copy
// cannot change the original.
func TestRedactedSharesNoLists(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	redacted := cfg.Redacted()

	redacted.Auth.JWT.Audience[0] = "changed"
	redacted.TLS.AllowedClientSANs[0] = "changed"

	assert.Equal(t, "gokvx", cfg.Auth.JWT.Audience[0])
	assert.Equal(t, "svc-microservice-1", cfg.TLS.AllowedClientSANs[0])
}

// TestLogValueIsRedacted_KV_CFG_006 checks that the log form of a
// configuration has one attribute per key and never carries URL credentials.
// Verifies: KV-CFG-006.
func TestLogValueIsRedacted_KV_CFG_006(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Auth.JWT.IssuerURL = "https://client:s3cret@issuer.test/"

	value := cfg.LogValue()

	assert.NotContains(t, value.String(), "s3cret")
	attrs := value.Group()
	require.NotEmpty(t, attrs)
	assert.Equal(t, "node.id", attrs[0].Key)
	assert.Equal(t, "gokvx-0", attrs[0].Value.String())
}

// TestYAMLRoundTrip checks that rendered YAML loads back to the same
// configuration, so the output of `gokvx config validate --print` is itself a
// valid configuration file.
func TestYAMLRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Storage.Fsync = config.FsyncInterval
	cfg.Cluster.InitialPeers = []string{"gokvx-1=gokvx-1:2380"}
	cfg.TLS.AllowedPeerSANs = []string{"gokvx-*.gokvx"}

	out, err := cfg.YAML()
	require.NoError(t, err)
	assert.Contains(t, string(out), "refresh_interval: 15m\n", "durations are written as in configuration files")

	path := filepath.Join(t.TempDir(), "rendered.yaml")
	require.NoError(t, os.WriteFile(path, out, 0o600))
	loaded, err := config.Load(config.Options{File: path})
	require.NoError(t, err)

	assert.Equal(t, cfg, loaded)
}
