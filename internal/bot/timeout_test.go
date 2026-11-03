package bot

import (
	"context"
	"testing"
	"time"
)

func TestRequestContextUsesConfiguredTimeout(t *testing.T) {
	b := &Bot{requestTimeout: 2 * time.Minute}
	ctx, cancel := b.requestContext(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected a deadline")
	}
	if left := time.Until(deadline); left < 119*time.Second || left > 2*time.Minute {
		t.Errorf("deadline in %v, want about 2m", left)
	}
}

func TestRequestContextFallsBackToDefault(t *testing.T) {
	b := &Bot{}
	ctx, cancel := b.requestContext(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected a deadline")
	}
	if left := time.Until(deadline); left < defaultRequestTimeout-time.Second || left > defaultRequestTimeout {
		t.Errorf("deadline in %v, want about %v", left, defaultRequestTimeout)
	}
}
