package app

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestSignalContextParentAndStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
	})
}
