package observability

import (
	"context"
	"io"
	"log/slog"

	"github.com/nightCode42/gokvx/internal/config"
	"github.com/nightCode42/gokvx/internal/kverr"
)

// NewLogger returns the node's logger and the variable holding its minimum
// level. The logger writes one event per line to w — JSON, or text when so
// configured in development (KV-OBS-020) — and adds node_id to every record
// (KV-OBS-021). Changing the returned LevelVar changes the level at once,
// without a restart (KV-OBS-022).
func NewLogger(w io.Writer, cfg config.Log, nodeID string) (*slog.Logger, *slog.LevelVar, error) {
	level := new(slog.LevelVar)
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return nil, nil, kverr.Wrap(kverr.ReasonInvalidArgument, err, "unknown log level: "+cfg.Level)
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default:
		return nil, nil, kverr.New(kverr.ReasonInvalidArgument, "unknown log format: "+cfg.Format)
	}

	logger := slog.New(contextHandler{handler}).With(slog.String("node_id", nodeID))
	return logger, level, nil
}

// contextKey identifies the request-scoped attributes stored in a context.
type contextKey struct{}

// WithAttrs returns a context whose log records carry attrs in addition to
// any already attached, such as request_id or principal set by the transport
// layer. Pass the context to the logger's *Context methods (InfoContext and
// so on) for the attributes to appear.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	existing, _ := ctx.Value(contextKey{}).([]slog.Attr)
	combined := make([]slog.Attr, 0, len(existing)+len(attrs))
	combined = append(combined, existing...)
	combined = append(combined, attrs...)
	return context.WithValue(ctx, contextKey{}, combined)
}

// contextHandler adds the attributes stored by WithAttrs to every record
// logged with that context.
type contextHandler struct {
	// Handler formats and writes the records.
	slog.Handler
}

// Handle adds the context's attributes to r, then passes it on.
func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(contextKey{}).([]slog.Attr); ok {
		r.AddAttrs(attrs...)
	}
	return h.Handler.Handle(ctx, r) //nolint:wrapcheck // a decorator must not change the handler's errors
}

// WithAttrs returns a handler that adds attrs to every record, keeping the
// context attributes.
func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

// WithGroup returns a handler that nests later attributes under name,
// keeping the context attributes.
func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}
