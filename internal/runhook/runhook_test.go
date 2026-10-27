package runhook

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestReadyStartWaitsForReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		h := NewReady("test", "r", func(ctx context.Context, ready func()) error {
			<-release
			ready()
			ready() // later calls are ignored
			<-ctx.Done()
			return nil
		})
		if err := h.Check(t.Context()); err == nil {
			t.Error("Check before Start = nil")
		}
		started := make(chan error, 1)
		go func() { started <- h.Start(t.Context()) }()
		synctest.Wait()
		select {
		case err := <-started:
			t.Fatalf("Start returned %v before ready", err)
		default:
		}
		if err := h.Check(t.Context()); err == nil {
			t.Error("Check while starting = nil")
		}
		close(release)
		if err := <-started; err != nil {
			t.Fatalf("Start() = %v", err)
		}
		if err := h.Check(t.Context()); err != nil {
			t.Errorf("Check after ready = %v", err)
		}
		if err := h.Stop(t.Context()); err != nil {
			t.Fatalf("Stop() = %v", err)
		}
		if _, ok := <-h.Err(); ok {
			t.Error("Err delivered a value after a clean stop")
		}
	})
}

func TestReadyStartReturnsEarlyEnd(t *testing.T) {
	errRun := errors.New("subscribe failed")
	for _, tc := range []struct {
		name string
		run  func(context.Context, func()) error
		want error
	}{
		{"error before ready", func(context.Context, func()) error { return errRun }, errRun},
		{"nil before ready", func(context.Context, func()) error { return nil }, ErrEnded},
		{"error right after ready", func(_ context.Context, ready func()) error { ready(); return errRun }, errRun},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := NewReady("test", "r", tc.run)
				err := h.Start(t.Context())
				synctest.Wait()
				if tc.name == "error right after ready" && err == nil {
					// The run may end after Start returned; it is then reported once through Err.
					err = <-h.Err()
				}
				if !errors.Is(err, tc.want) {
					t.Fatalf("Start() = %v, want %v", err, tc.want)
				}
				if got, ok := <-h.Err(); ok {
					t.Errorf("failure reported twice, Err delivered %v", got)
				}
				if err := h.Check(t.Context()); err == nil {
					t.Error("Check after the run ended = nil")
				}
				if err := h.Stop(t.Context()); err != nil {
					t.Errorf("Stop() = %v", err)
				}
			})
		})
	}
}

func TestReadyStartBoundedByContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var canceled bool
		h := NewReady("test", "r", func(ctx context.Context, _ func()) error {
			<-ctx.Done()
			canceled = true
			return ctx.Err()
		})
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		err := h.Start(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Start() = %v, want DeadlineExceeded", err)
		}
		if !canceled {
			t.Error("Start returned before the run function returned")
		}
		if _, ok := <-h.Err(); ok {
			t.Error("Err delivered the cancellation")
		}
		if err := h.Start(t.Context()); err == nil {
			t.Error("second Start succeeded")
		}
	})
}

func TestStartWithoutReadyReturnsImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := New("test", "n", func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
		if err := h.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := h.Check(t.Context()); err != nil {
			t.Errorf("Check() = %v", err)
		}
		if err := h.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := h.Check(t.Context()); err == nil {
			t.Error("Check after Stop = nil")
		}
	})
}

func TestRuntimeEndAfterReadyIsReported(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fail := make(chan struct{})
		h := NewReady("test", "r", func(_ context.Context, ready func()) error {
			ready()
			<-fail
			return nil
		})
		if err := h.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		close(fail)
		if err := <-h.Err(); !errors.Is(err, ErrEnded) {
			t.Fatalf("Err() = %v, want ErrEnded", err)
		}
	})
}
