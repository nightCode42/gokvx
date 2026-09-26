package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/config"
	"github.com/nightCode42/gokvx/internal/kverr"
	"github.com/nightCode42/gokvx/internal/observability"
)

// newTestLogger returns a logger writing to a buffer, and the buffer.
func newTestLogger(t *testing.T, cfg config.Log) (*slog.Logger, *slog.LevelVar, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	logger, level, err := observability.NewLogger(&buf, cfg, "gokvx-0")
	require.NoError(t, err)
	return logger, level, &buf
}

// records decodes one JSON object per line.
func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec), "not one JSON object per line: %q", line)
		out = append(out, rec)
	}
	return out
}

// TestLoggerWritesJSONLines_KV_OBS_020 checks the format: one JSON object per
// line, with level, message, and node_id on every record.
// Verifies: KV-OBS-020, KV-OBS-021.
func TestLoggerWritesJSONLines_KV_OBS_020(t *testing.T) {
	t.Parallel()

	logger, _, buf := newTestLogger(t, config.Log{Level: "info", Format: "json"})

	logger.Info("node starting", slog.Int("port", 2379))
	logger.Warn("fsync relaxed")

	recs := records(t, buf)
	require.Len(t, recs, 2)
	assert.Equal(t, "INFO", recs[0]["level"])
	assert.Equal(t, "node starting", recs[0]["msg"])
	assert.InDelta(t, 2379, recs[0]["port"], 0)
	for _, rec := range recs {
		assert.Equal(t, "gokvx-0", rec["node_id"])
	}
}

// TestLoggerLevelChangesAtRuntime_KV_OBS_022 checks the minimum level, and that
// changing it takes effect without rebuilding the logger.
// Verifies: KV-OBS-022, KV-OBS-023.
func TestLoggerLevelChangesAtRuntime_KV_OBS_022(t *testing.T) {
	t.Parallel()

	logger, level, buf := newTestLogger(t, config.Log{Level: "info", Format: "json"})

	logger.Debug("per-request detail")
	assert.Empty(t, buf.String(), "debug is suppressed at info")

	level.Set(slog.LevelDebug)
	logger.Debug("per-request detail")
	assert.Len(t, records(t, buf), 1)
}

// TestLoggerAddsContextAttributes_KV_OBS_021 checks that request-scoped fields
// attached to a context appear on records logged with it, and survive
// derived loggers.
// Verifies: KV-OBS-021.
func TestLoggerAddsContextAttributes_KV_OBS_021(t *testing.T) {
	t.Parallel()

	logger, _, buf := newTestLogger(t, config.Log{Level: "info", Format: "json"})

	ctx := observability.WithAttrs(context.Background(),
		slog.String("request_id", "req-1"), slog.String("principal", "svc-a"))
	ctx = observability.WithAttrs(ctx, slog.String("method", "/gokvx.v1.KVService/Get"))

	logger.With("component", "server").InfoContext(ctx, "handled")
	logger.InfoContext(context.Background(), "unrelated")

	recs := records(t, buf)
	require.Len(t, recs, 2)
	assert.Equal(t, "req-1", recs[0]["request_id"])
	assert.Equal(t, "svc-a", recs[0]["principal"])
	assert.Equal(t, "/gokvx.v1.KVService/Get", recs[0]["method"])
	assert.Equal(t, "server", recs[0]["component"])
	assert.NotContains(t, recs[1], "request_id")
}

// TestLoggerGroupsKeepContextAttributes checks that a grouped logger still
// receives the context's attributes.
func TestLoggerGroupsKeepContextAttributes(t *testing.T) {
	t.Parallel()

	logger, _, buf := newTestLogger(t, config.Log{Level: "info", Format: "json"})
	ctx := observability.WithAttrs(context.Background(), slog.String("request_id", "req-2"))

	logger.WithGroup("raft").InfoContext(ctx, "term changed", slog.Int("term", 3))

	rec := records(t, buf)[0]
	group, ok := rec["raft"].(map[string]any)
	require.True(t, ok, "record: %v", rec)
	assert.InDelta(t, 3, group["term"], 0)
	assert.Equal(t, "req-2", group["request_id"])
}

// TestLoggerTextFormat checks the development-only text format.
func TestLoggerTextFormat(t *testing.T) {
	t.Parallel()

	logger, _, buf := newTestLogger(t, config.Log{Level: "info", Format: "text"})

	logger.Info("hello")

	assert.Contains(t, buf.String(), "level=INFO msg=hello node_id=gokvx-0")
}

// TestNewLoggerRejectsUnknownSettings checks the defensive checks behind the
// configuration's own validation.
func TestNewLoggerRejectsUnknownSettings(t *testing.T) {
	t.Parallel()

	for name, cfg := range map[string]config.Log{
		"level":  {Level: "trace", Format: "json"},
		"format": {Level: "info", Format: "xml"},
	} {
		_, _, err := observability.NewLogger(&bytes.Buffer{}, cfg, "gokvx-0")

		assert.Equal(t, kverr.ReasonInvalidArgument, kverr.ReasonOf(err), name)
	}
}

// TestLoggingAnErrorUsesItsLogValue checks that a kverr error logs as a group
// of reason, kind, message, and cause.
func TestLoggingAnErrorUsesItsLogValue(t *testing.T) {
	t.Parallel()

	logger, _, buf := newTestLogger(t, config.Log{Level: "info", Format: "json"})
	err := kverr.Wrap(kverr.ReasonInternal, context.DeadlineExceeded, "command log sync failed")

	logger.Error("request failed", slog.Any("error", err))

	group, ok := records(t, buf)[0]["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{
		"reason":  "INTERNAL",
		"kind":    "INTERNAL",
		"message": "command log sync failed",
		"cause":   "context deadline exceeded",
	}, group)
}

// TestLoggingAConfigurationRedactsIt_KV_CFG_006 checks that a configuration
// logged directly, by value, never shows credentials embedded in URLs.
// Verifies: KV-CFG-006.
func TestLoggingAConfigurationRedactsIt_KV_CFG_006(t *testing.T) {
	t.Parallel()

	logger, _, buf := newTestLogger(t, config.Log{Level: "info", Format: "json"})
	cfg := config.Defaults()
	cfg.Auth.JWT.JWKSURL = "https://client:s3cret@issuer.test/jwks.json"

	logger.Info("effective configuration", slog.Any("config", cfg))

	assert.NotContains(t, buf.String(), "s3cret")
	group, ok := records(t, buf)[0]["config"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://REDACTED@issuer.test/jwks.json", group["auth.jwt.jwks_url"])
	assert.Equal(t, "1m", group["auth.jwt.clock_skew"], "durations are written as in configuration")
	assert.Equal(t, "always", group["storage.fsync"])
}
