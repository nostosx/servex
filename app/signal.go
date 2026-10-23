package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// abortKey carries, in a context returned by SignalContext, a context that is
// cancelled when the second signal arrives.
type abortKey struct{}

// SignalContext returns a copy of parent that is cancelled when the first of
// sig arrives; context.Cause reports the signal. A second signal makes an
// App.Run driven by the returned context abandon the rest of its stop
// procedure immediately. Without sig, os.Interrupt and syscall.SIGTERM are
// used.
//
// Signal handling stays installed until the returned CancelFunc is called, so
// that the second signal can still be observed while the App is stopping.
// Calling the CancelFunc cancels the context (without aborting a stop in
// progress) and restores default signal behaviour.
func SignalContext(parent context.Context, sig ...os.Signal) (context.Context, context.CancelFunc) {
	if len(sig) == 0 {
		sig = []os.Signal{os.Interrupt, syscall.SIGTERM}
	}
	ctx, cancel := context.WithCancelCause(parent)
	abortCtx, abort := context.WithCancel(context.Background())

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, sig...)
	done := make(chan struct{})
	go func() {
		defer signal.Stop(ch)
		// abort is deliberately not deferred: releasing the context must not
		// abandon a graceful stop that is still in progress.
		select {
		case s := <-ch:
			cancel(fmt.Errorf("app: received signal %v", s))
		case <-ctx.Done():
			// The parent ended without a signal. The next signal is still the
			// first one: it must not abort the graceful stop it would trigger.
			select {
			case <-ch:
			case <-done:
				return
			}
		}
		select {
		case <-ch:
			abort()
		case <-done:
		}
	}()

	release := sync.OnceFunc(func() {
		close(done)
		cancel(context.Canceled)
	})
	return context.WithValue(ctx, abortKey{}, abortCtx), release
}
