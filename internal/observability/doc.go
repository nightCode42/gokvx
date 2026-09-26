// Package observability provides the node's structured logging (spec §12.3)
// and, in later phases, its tracing and metrics.
//
// Logs are JSON, one event per line, written through log/slog (KV-OBS-020).
// Every record carries node_id, and request-scoped fields — trace_id,
// span_id, request_id, principal, method — are attached through the context
// with [WithAttrs], so each log line of a request can be correlated with its
// trace (KV-OBS-021). The minimum level is held in a slog.LevelVar that can
// be changed while the node runs (KV-OBS-022).
//
// Loggers are passed to components explicitly; this package holds no global
// logger.
package observability
