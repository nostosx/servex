package runhook

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// ErrEnded is reported when the run function returns nil before Stop: a
// consumer or worker that silently stops consuming must fail the App rather
// than leave it running and ready.
var ErrEnded = errors.New("run ended unexpectedly")

// Hook runs a blocking function in the background between Start and Stop. It
// implements app.Hook and the optional Err() <-chan error structurally.
type Hook struct {
	prefix string
	name   string
	run    func(ctx context.Context) error
	errc   chan error

	mu       sync.Mutex
	started  bool
	cancel   context.CancelFunc
	done     chan struct{}
	stopping atomic.Bool
}

// New returns a Hook named name. prefix (such as "broker") starts every error
// message. run must block until its ctx is canceled or a fatal error occurs.
func New(prefix, name string, run func(ctx context.Context) error) *Hook {
	return &Hook{prefix: prefix, name: name, run: run, errc: make(chan error, 1)}
}

// Name returns the hook name.
func (h *Hook) Name() string { return h.name }

// Start launches the run function in a new goroutine and returns immediately.
// The run context keeps the values of ctx but is cancelled only by Stop. A
// Hook can be started only once.
func (h *Hook) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started {
		return fmt.Errorf("%s: %s already started", h.prefix, h.name)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	h.started, h.cancel, h.done = true, cancel, make(chan struct{})
	go func() {
		defer close(h.done)
		defer close(h.errc)
		err := h.run(runCtx)
		if h.stopping.Load() && (err == nil || errors.Is(err, context.Canceled)) {
			return
		}
		if err == nil {
			err = ErrEnded
		}
		h.errc <- fmt.Errorf("%s: %s: %w", h.prefix, h.name, err)
	}()
	return nil
}

// Stop cancels the run function and waits for it to return, bounded by ctx.
// It returns nil when the hook was never started.
func (h *Hook) Stop(ctx context.Context) error {
	h.mu.Lock()
	cancel, done := h.cancel, h.done
	h.mu.Unlock()
	if done == nil {
		return nil
	}
	h.stopping.Store(true)
	cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return fmt.Errorf("%s: stop %s: %w", h.prefix, h.name, ctx.Err())
	}
	// An error nobody has received yet (for example because the run function
	// failed while stopping) is returned here instead of being lost.
	select {
	case err := <-h.errc:
		return err
	default:
		return nil
	}
}

// Err reports the failure of the run function: a non-nil error, or ErrEnded
// when it returned nil before Stop. The channel is closed once the run
// function has returned.
func (h *Hook) Err() <-chan error { return h.errc }
