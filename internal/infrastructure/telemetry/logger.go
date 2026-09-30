package telemetry

import (
	"context"
	"log/slog"
	"os"
)

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
)

// ContextHandler wraps an slog.Handler to inject context values (such as request_id) into records.
type ContextHandler struct {
	slog.Handler
}

// Handle extracts context values and appends them as slog attributes.
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
		r.AddAttrs(slog.String("request_id", reqID))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs returns a new ContextHandler with additional attributes.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup returns a new ContextHandler with a given group name.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithGroup(name)}
}

// NewLogger initializes a production-grade slog.Logger.
func NewLogger(isJSON bool, level slog.Level) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	if isJSON {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	wrappedHandler := &ContextHandler{Handler: handler}
	return slog.New(wrappedHandler)
}

// WithRequestID returns a new context carrying the given request ID.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID retrieves the request ID from context if present.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}
