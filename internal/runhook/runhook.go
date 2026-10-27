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
// implements app.Hook and the optional Err() <-chan error and
// Check(ctx) error structurally.
type Hook struct {
	prefix    string
	name      string
	run       func(ctx context.Context, ready func()) error
	waitReady bool
	errc      chan error

	mu       sync.Mutex
	started  bool
	starting bool  // Start is waiting for readiness
	ended    bool  // the run function has returned
	endErr   error // its failure, nil after a clean stop
	cancel   context.CancelFunc
	done     chan struct{}
	stopping atomic.Bool
}

// New returns a Hook named name whose Start returns as soon as run is
// launched. prefix (such as "broker") starts every error message. run must
// block until its ctx is canceled or a fatal error occurs.
func New(prefix, name string, run func(ctx context.Context) error) *Hook {
	return &Hook{
		prefix: prefix,
		name:   name,
		run:    func(ctx context.Context, _ func()) error { return run(ctx) },
		errc:   make(chan error, 1),
	}
}

// NewReady returns a Hook like New whose Start waits until run calls ready.
// run may call ready any number of times; only the first call counts.
func NewReady(prefix, name string, run func(ctx context.Context, ready func()) error) *Hook {
	return &Hook{prefix: prefix, name: name, run: run, waitReady: true, errc: make(chan error, 1)}
}

// Name returns the hook name.
func (h *Hook) Name() string { return h.name }

// Start launches the run function in a new goroutine. The run context keeps
// the values of ctx but is cancelled only by Stop. A Hook can be started only
// once.
//
// A Hook created with New returns immediately. A Hook created with NewReady
// waits until the run function reports readiness, returns or ctx ends. If the
// run function returns first, Start returns its error, or ErrEnded for nil,
// and the failure is not reported again through Err. If ctx ends first, Start
// cancels the run function, waits for it to return and returns the context
// error.
func (h *Hook) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.started {
		h.mu.Unlock()
		return fmt.Errorf("%s: %s already started", h.prefix, h.name)
	}
	if err := ctx.Err(); err != nil {
		h.mu.Unlock()
		return err
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	h.started, h.starting, h.cancel, h.done = true, h.waitReady, cancel, done
	h.mu.Unlock()

	readyc := make(chan struct{})
	var once sync.Once
	ready := func() { once.Do(func() { close(readyc) }) }
	go func() {
		err := h.run(runCtx, ready)
		h.finish(err)
		close(h.errc)
		close(done)
	}()
	if !h.waitReady {
		return nil
	}

	var startErr error
	select {
	case <-readyc:
	case <-done:
	case <-ctx.Done():
		startErr = fmt.Errorf("%s: start %s: %w", h.prefix, h.name, context.Cause(ctx))
		h.stopping.Store(true)
		cancel()
		<-done
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.starting = false
	if startErr != nil {
		return errors.Join(startErr, h.endErr)
	}
	if h.ended {
		// finish saw starting set and left the failure to Start.
		if h.endErr != nil {
			return h.endErr
		}
		return h.wrap(ErrEnded)
	}
	return nil
}

// finish records the outcome of the run function and reports a failure
// through errc unless Start is still waiting for readiness and returns it.
func (h *Hook) finish(err error) {
	if h.stopping.Load() && (err == nil || errors.Is(err, context.Canceled)) {
		err = nil
	} else {
		if err == nil {
			err = ErrEnded
		}
		err = h.wrap(err)
	}
	h.mu.Lock()
	h.ended, h.endErr = true, err
	report := !h.starting
	h.mu.Unlock()
	if report && err != nil {
		h.errc <- err
	}
}

// wrap adds the hook prefix and name to err.
func (h *Hook) wrap(err error) error {
	return fmt.Errorf("%s: %s: %w", h.prefix, h.name, err)
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

// Err reports the failure of the run function after Start returned: a
// non-nil error, or ErrEnded when it returned nil before Stop. The channel is
// closed once the run function has returned.
func (h *Hook) Err() <-chan error { return h.errc }

// Check reports readiness: an error before Start has returned successfully
// and once the run function has returned, nil otherwise.
func (h *Hook) Check(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case !h.started:
		return fmt.Errorf("%s: %s not started", h.prefix, h.name)
	case h.starting:
		return fmt.Errorf("%s: %s not ready", h.prefix, h.name)
	case h.ended:
		return fmt.Errorf("%s: %s not running", h.prefix, h.name)
	default:
		return nil
	}
}
