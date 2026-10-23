package app

import (
	"context"
	"testing"
)

func TestSignalContextParentAndStop(t *testing.T) {
	// Signal registration uses process-wide runtime state outside synctest.
	parent, cancel := context.WithCancel(t.Context())
	ctx, stop := SignalContext(parent)
	defer stop()
	cancel()
	if cause := context.Cause(ctx); cause != context.Canceled {
		t.Fatalf("parent cancellation cause = %v", cause)
	}

	ctx2, stop2 := SignalContext(t.Context())
	stop2()
	stop2() // idempotent
	if cause := context.Cause(ctx2); cause != context.Canceled {
		t.Fatalf("stop cancellation cause = %v", cause)
	}
}
