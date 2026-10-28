package sentrylog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

// captureTransport is a sentry Transport that records events instead of sending them.
type captureTransport struct {
	events []*sentry.Event
}

func (t *captureTransport) Configure(sentry.ClientOptions) {}
func (t *captureTransport) SendEvent(event *sentry.Event)  { t.events = append(t.events, event) }
func (t *captureTransport) Flush(time.Duration) bool       { return true }
func (t *captureTransport) FlushWithContext(context.Context) bool {
	return true
}
func (t *captureTransport) Close() {}

func newTestHub(t *testing.T) (*sentry.Hub, *captureTransport) {
	t.Helper()
	transport := &captureTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       "https://key@example.com/1",
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return sentry.NewHub(client, sentry.NewScope()), transport
}

func TestHandlerCapturesErrorLevel(t *testing.T) {
	hub, transport := newTestHub(t)
	h := NewHandler(slog.NewTextHandler(io.Discard, nil), hub)
	logger := slog.New(h)

	logger.Error("boom", "err", errors.New("disk full"))
	hub.Flush(0)

	if len(transport.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(transport.events))
	}
	ev := transport.events[0]
	if len(ev.Exception) == 0 {
		t.Fatalf("expected exception in event, got none")
	}
	if ev.Exception[len(ev.Exception)-1].Value != "disk full" {
		t.Errorf("exception value = %q", ev.Exception[len(ev.Exception)-1].Value)
	}
}

func TestHandlerCapturesMessageWithoutError(t *testing.T) {
	hub, transport := newTestHub(t)
	h := NewHandler(slog.NewTextHandler(io.Discard, nil), hub)
	logger := slog.New(h)

	logger.Error("something bad happened")
	hub.Flush(0)

	if len(transport.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(transport.events))
	}
	if transport.events[0].Message != "something bad happened" {
		t.Errorf("message = %q", transport.events[0].Message)
	}
}

func TestHandlerIgnoresBelowError(t *testing.T) {
	hub, transport := newTestHub(t)
	h := NewHandler(slog.NewTextHandler(io.Discard, nil), hub)
	logger := slog.New(h)

	logger.Info("just info")
	logger.Warn("a warning")
	hub.Flush(0)

	if len(transport.events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(transport.events))
	}
}

func TestHandlerDelegatesToInner(t *testing.T) {
	hub, _ := newTestHub(t)
	inner := &countingHandler{}
	h := NewHandler(inner, hub)
	logger := slog.New(h)

	logger.Info("one")
	logger.Error("two")

	if inner.count != 2 {
		t.Errorf("inner handled %d records, want 2", inner.count)
	}
}

type countingHandler struct {
	count int
}

func (c *countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (c *countingHandler) Handle(context.Context, slog.Record) error {
	c.count++
	return nil
}
func (c *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *countingHandler) WithGroup(string) slog.Handler      { return c }
