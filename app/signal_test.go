//go:build unix

package app

import (
	"context"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Signal tests use real signals delivered to the test process, so they cannot
// run inside a synctest bubble. signalWait only guards against hangs; the
// parent-cancellation test also uses a quiet window to observe real signal delivery.
const signalWait = 10 * time.Second

func waitDone(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(signalWait):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func kill(t *testing.T) {
	t.Helper()
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatalf("kill: %v", err)
	}
}

func TestSignalContextFirstSignalCancels(t *testing.T) {
	ctx, stop := SignalContext(t.Context(), syscall.SIGUSR1)
	defer stop()
	if err := ctx.Err(); err != nil {
		t.Fatalf("ctx.Err() before signal = %v", err)
	}
	kill(t)
	waitDone(t, ctx.Done(), "cancellation")
	if cause := context.Cause(ctx); cause == nil || !strings.Contains(cause.Error(), "app: received signal") {
		t.Fatalf("context.Cause = %v", cause)
	}
}

func TestSignalContextSecondSignalAbortsRun(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{})
	stopping := make(chan struct{})
	r := &recorder{}
	stuck := NewHook("stuck",
		func(context.Context) error { close(started); return nil },
		func(context.Context) error { close(stopping); <-release; return nil },
	)
	a := New(quiet(), WithStopTimeout(0), WithHooks(recHook(r, "first", nil, nil), stuck))

	ctx, stop := SignalContext(t.Context(), syscall.SIGUSR1)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	waitDone(t, started, "start")
	kill(t) // first signal: graceful stop begins
	waitDone(t, stopping, "hook stop")
	select {
	case err := <-done:
		t.Fatalf("Run() returned before second signal: %v", err)
	default:
	}
	kill(t) // second signal: abandon the stop

	select {
	case err := <-done:
		if err == nil || err.Error() != "app: stop timeout, unfinished hooks: [stuck first]" {
			t.Fatalf("Run() = %v", err)
		}
	case <-time.After(signalWait):
		t.Fatal("Run() did not return after second signal")
	}
	if s := a.State(); s != StateFailed {
		t.Fatalf("state = %v, want failed", s)
	}
	if got := r.get(); len(got) != 1 {
		t.Fatalf("events = %v, want only start:first", got)
	}
}

func TestSignalContextFirstSignalAfterParentCancelDoesNotAbort(t *testing.T) {
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	started := make(chan struct{})
	stopping := make(chan struct{})
	slow := NewHook("slow",
		func(context.Context) error { close(started); return nil },
		func(context.Context) error { close(stopping); <-release; return nil },
	)
	a := New(quiet(), WithStopTimeout(0), WithHooks(slow))

	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx, stop := SignalContext(parent, syscall.SIGUSR1)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	waitDone(t, started, "start")
	cancel() // shutdown triggered by the parent, not by a signal
	waitDone(t, stopping, "hook stop")
	kill(t) // the first signal must not abandon the graceful stop
	select {
	case err := <-done:
		t.Fatalf("Run() returned after a single signal: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
	case <-time.After(signalWait):
		t.Fatal("Run() did not return")
	}
}

func TestSignalContextGracefulRun(t *testing.T) {
	started := make(chan struct{})
	a := New(quiet(), WithHooks(NewHook("h", func(context.Context) error { close(started); return nil }, nil)))
	ctx, stop := SignalContext(t.Context(), syscall.SIGUSR1)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	waitDone(t, started, "start")
	kill(t)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
	case <-time.After(signalWait):
		t.Fatal("Run() did not return after signal")
	}
	if s := a.State(); s != StateStopped {
		t.Fatalf("state = %v, want stopped", s)
	}
}
