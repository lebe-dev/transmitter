// Package sentrylog wires the Sentry SDK into the application for error
// reporting only. Performance tracing and profiling are intentionally
// disabled — Sentry receives just error events with their stack traces.
package sentrylog

import (
	"context"
	"log/slog"
	"time"

	"github.com/getsentry/sentry-go"
)

// Init initializes the global Sentry SDK with error reporting only.
// dsn must be non-empty; environment and release are attached to every event.
func Init(dsn, environment, release string) error {
	return sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      environment,
		Release:          release,
		AttachStacktrace: true,
		EnableTracing:    false,
		TracesSampleRate: 0,
	})
}

// Flush waits up to timeout for buffered events to be delivered to Sentry.
func Flush(timeout time.Duration) bool {
	return sentry.Flush(timeout)
}

// Handler is a slog.Handler that forwards Error-level (and above) records to
// Sentry while delegating every record to a wrapped handler.
type Handler struct {
	inner slog.Handler
	hub   *sentry.Hub
	attrs []slog.Attr
}

// NewHandler wraps inner so that Error-level records are reported to Sentry via
// hub. If hub is nil, the current hub is used.
func NewHandler(inner slog.Handler, hub *sentry.Hub) *Handler {
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	return &Handler{inner: inner, hub: hub}
}

// Enabled reports whether the wrapped handler handles records at the given level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle reports Error-level records to Sentry and always delegates to the
// wrapped handler.
func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level >= slog.LevelError {
		h.capture(record)
	}
	return h.inner.Handle(ctx, record)
}

// WithAttrs returns a new Handler whose records carry the given attributes.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &Handler{inner: h.inner.WithAttrs(attrs), hub: h.hub, attrs: merged}
}

// WithGroup returns a new Handler that delegates grouping to the wrapped handler.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{inner: h.inner.WithGroup(name), hub: h.hub, attrs: h.attrs}
}

// capture builds a Sentry event from the record. If an "err" attribute holds an
// error, it is reported as an exception (preserving its stack trace); otherwise
// the log message is reported as a message event.
func (h *Handler) capture(record slog.Record) {
	var err error
	extra := make(map[string]any)

	collect := func(a slog.Attr) {
		if a.Key == "err" {
			if e, ok := a.Value.Any().(error); ok {
				err = e
				return
			}
		}
		extra[a.Key] = a.Value.Any()
	}

	for _, a := range h.attrs {
		collect(a)
	}
	record.Attrs(func(a slog.Attr) bool {
		collect(a)
		return true
	})

	h.hub.WithScope(func(scope *sentry.Scope) {
		if err != nil && record.Message != "" {
			extra["log_message"] = record.Message
		}
		if len(extra) > 0 {
			scope.SetContext("log", sentry.Context(extra))
		}
		if err != nil {
			h.hub.CaptureException(err)
			return
		}
		h.hub.CaptureMessage(record.Message)
	})
}

// ensure Handler implements slog.Handler.
var _ slog.Handler = (*Handler)(nil)
