package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// defaultStopTimeout bounds Stop when WithStopTimeout is not given.
const defaultStopTimeout = 30 * time.Second

// Option configures an App.
type Option func(*options)

type options struct {
	hooks       []Hook
	logger      *slog.Logger
	stopDelay   time.Duration
	stopTimeout time.Duration
}

// WithHooks appends hooks to the App. Hooks start in the order they are
// registered and stop in reverse order. Nil hooks are ignored.
func WithHooks(hs ...Hook) Option {
	return func(o *options) {
		for _, h := range hs {
			if h != nil {
				o.hooks = append(o.hooks, h)
			}
		}
	}
}

// WithLogger sets the logger used for lifecycle events. The default is
// slog.Default(); a nil logger keeps the default.
func WithLogger(l *slog.Logger) Option {
	return func(o *options) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithStopDelay sets how long Stop waits after entering StateStopping (and
// thus failing readiness) before stopping any hook, giving load balancers
// time to drain traffic. The default is 0.
func WithStopDelay(d time.Duration) Option {
	return func(o *options) { o.stopDelay = max(d, 0) }
}

// WithStopTimeout bounds the whole of Stop, including the stop delay. The
// default is 30s; a value <= 0 means no limit beyond the caller's context.
func WithStopTimeout(d time.Duration) Option {
	return func(o *options) { o.stopTimeout = d }
}

// App runs a list of Hooks through a single lifecycle. All methods are safe
// for concurrent use; hooks must support readiness checks during lifecycle
// transitions. Create an App with New; its zero value is not usable.
// App satisfies health.Checker structurally through Check.
type App struct {
	hooks       []Hook
	logger      *slog.Logger
	stopDelay   time.Duration
	stopTimeout time.Duration

	started chan struct{} // closed when Start returns
	stopped chan struct{} // closed when the first effective Stop finishes
	fatal   chan struct{} // closed on the first runtime failure
	quit    chan struct{} // closed when stopping begins; ends Err watchers

	watchers  sync.WaitGroup
	fatalOnce sync.Once

	mu       sync.Mutex
	state    State
	managed  bool // true when driven by Run, which performs the stop itself
	stopErr  error
	failures []error
}

// New creates an App in StateInit.
func New(opts ...Option) *App {
	o := options{logger: slog.Default(), stopTimeout: defaultStopTimeout}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return &App{
		hooks:       o.hooks,
		logger:      o.logger,
		stopDelay:   o.stopDelay,
		stopTimeout: o.stopTimeout,
		started:     make(chan struct{}),
		stopped:     make(chan struct{}),
		fatal:       make(chan struct{}),
		quit:        make(chan struct{}),
	}
}

// State returns the current lifecycle state.
func (a *App) State() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

// Start starts every hook in registration order. It is only allowed in
// StateInit; otherwise it returns an error wrapping ErrInvalidState.
//
// If the i-th hook fails to start, hooks [0, i) are stopped in reverse order
// (bounded by the stop timeout), the App ends in StateFailed and the start
// error ("app: start hook %q: %w") is joined with any rollback errors
// ("app: stop hook %q: %w"). On success the App enters StateRunning and
// starts watching the hooks' Err channels.
func (a *App) Start(ctx context.Context) error {
	return a.start(ctx, false)
}

func (a *App) start(ctx context.Context, managed bool) error {
	a.mu.Lock()
	if a.state != StateInit {
		s := a.state
		a.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrInvalidState, s)
	}
	a.state = StateStarting
	a.managed = managed
	a.mu.Unlock()
	defer close(a.started)

	a.logger.Info("app starting", "hooks", len(a.hooks))
	for i, h := range a.hooks {
		err := a.startHook(ctx, h)
		if err == nil {
			continue
		}
		stopCtx, cancel := a.stopContext(context.WithoutCancel(ctx))
		errs := append([]error{err}, a.stopHooks(stopCtx, a.hooks[:i])...)
		cancel()
		a.setState(StateFailed)
		a.logger.Error("app start failed", "state", StateFailed)
		return errors.Join(errs...)
	}

	a.mu.Lock()
	for _, h := range a.hooks {
		if r, ok := h.(errReporter); ok {
			if ch := r.Err(); ch != nil {
				a.watchers.Go(func() { a.watch(h, ch) })
			}
		}
	}
	a.state = StateRunning
	a.mu.Unlock()
	a.logger.Info("app running")
	return nil
}

func (a *App) startHook(ctx context.Context, h Hook) error {
	name := h.Name()
	begin := time.Now()
	err := ctx.Err()
	if err == nil {
		err = h.Start(ctx)
	}
	elapsed := time.Since(begin)
	if err != nil {
		a.logger.Error("hook start failed", "hook", name, "duration", elapsed, "error", err)
		return fmt.Errorf("app: start hook %q: %w", name, err)
	}
	a.logger.Info("hook started", "hook", name, "duration", elapsed)
	return nil
}

// watch forwards runtime failures reported by h until stopping begins or the
// channel is closed.
func (a *App) watch(h Hook, errs <-chan error) {
	for {
		select {
		case <-a.quit:
			return
		case err, ok := <-errs:
			if !ok {
				return
			}
			if err != nil {
				a.fail(h.Name(), err)
			}
		}
	}
}

// fail records a runtime failure and triggers the overall stop.
func (a *App) fail(name string, err error) {
	a.logger.Error("hook failed", "hook", name, "error", err)
	a.mu.Lock()
	a.failures = append(a.failures, fmt.Errorf("app: hook %q: %w", name, err))
	managed := a.managed
	a.mu.Unlock()
	a.fatalOnce.Do(func() {
		close(a.fatal)
		if !managed {
			// Nobody is waiting in Run, so stop on our own. Callers of Stop
			// observe the same result because Stop is idempotent, so the
			// result is not needed here.
			go func() { _ = a.Stop(context.Background()) }()
		}
	})
}

// Stop stops the App. In StateInit it moves directly to StateStopped and
// returns nil. In StateRunning it enters StateStopping (from which point
// Check fails), waits for the stop delay and then stops the hooks in reverse
// order, each in its own goroutine. The whole procedure is bounded by
// min(ctx, stop timeout); on expiry Stop stops waiting, calls no further hooks
// and reports "app: stop timeout, unfinished hooks: [...]".
//
// The App ends in StateFailed if any hook failed to stop or a runtime failure
// was reported, otherwise in StateStopped. Stop is idempotent: concurrent and
// repeated calls return the result of the first one. If Start is still in
// progress, Stop waits for it first; after a failed Start, Stop returns nil.
// A caller whose ctx ends while waiting for another caller's Stop gets the
// context error instead.
// A hook that ignores its stop context may continue running after Stop returns.
func (a *App) Stop(ctx context.Context) error {
	for {
		a.mu.Lock()
		switch a.state {
		case StateInit:
			a.state = StateStopped
			a.mu.Unlock()
			a.logger.Info("app stopped", "state", StateStopped)
			return nil
		case StateStarting:
			a.mu.Unlock()
			select {
			case <-a.started:
			case <-ctx.Done():
				return fmt.Errorf("app: stop: %w", context.Cause(ctx))
			}
		case StateRunning:
			a.state = StateStopping
			a.mu.Unlock()
			err := a.stop(ctx)
			a.mu.Lock()
			a.stopErr = err
			a.state = StateStopped
			if err != nil || len(a.failures) > 0 {
				a.state = StateFailed
			}
			final := a.state
			close(a.stopped)
			a.mu.Unlock()
			if final == StateFailed {
				a.logger.Error("app stopped", "state", final)
			} else {
				a.logger.Info("app stopped", "state", final)
			}
			return err
		case StateStopping:
			a.mu.Unlock()
			select {
			case <-a.stopped:
			case <-ctx.Done():
				return fmt.Errorf("app: stop: %w", context.Cause(ctx))
			}
		default: // StateStopped, StateFailed
			err := a.stopErr
			a.mu.Unlock()
			return err
		}
	}
}

func (a *App) stop(ctx context.Context) error {
	close(a.quit)
	a.watchers.Wait()

	a.logger.Info("app stopping", "stop_delay", a.stopDelay)
	ctx, cancel := a.stopContext(ctx)
	defer cancel()
	if a.stopDelay > 0 {
		select {
		case <-time.After(a.stopDelay):
		case <-ctx.Done():
		}
	}
	return errors.Join(a.stopHooks(ctx, a.hooks)...)
}

// stopContext bounds ctx by the configured stop timeout, if any.
func (a *App) stopContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if a.stopTimeout > 0 {
		return context.WithTimeout(ctx, a.stopTimeout)
	}
	return context.WithCancel(ctx)
}

// stopHooks stops hooks in reverse order. Each Stop runs in its own goroutine
// so a hook that ignores ctx cannot block the App past ctx's end.
func (a *App) stopHooks(ctx context.Context, hooks []Hook) []error {
	var errs []error
	for i, h := range slices.Backward(hooks) {
		if ctx.Err() != nil {
			return append(errs, a.timeoutError(hooks[:i+1]))
		}
		name := h.Name()
		begin := time.Now()
		done := make(chan error, 1)
		go func() { done <- h.Stop(ctx) }()
		var err error
		select {
		case err = <-done:
		case <-ctx.Done():
			select {
			case err = <-done:
			default:
				return append(errs, a.timeoutError(hooks[:i+1]))
			}
		}
		elapsed := time.Since(begin)
		if err != nil {
			a.logger.Error("hook stop failed", "hook", name, "duration", elapsed, "error", err)
			errs = append(errs, fmt.Errorf("app: stop hook %q: %w", name, err))
			continue
		}
		a.logger.Info("hook stopped", "hook", name, "duration", elapsed)
	}
	return errs
}

// timeoutError reports the hooks that were not stopped, in stop order.
func (a *App) timeoutError(unfinished []Hook) error {
	names := make([]string, 0, len(unfinished))
	for _, h := range slices.Backward(unfinished) {
		names = append(names, h.Name())
	}
	a.logger.Error("app stop timeout", "unfinished", names)
	return fmt.Errorf("app: stop timeout, unfinished hooks: %v", names)
}

// Run starts the App, waits until ctx is done, a hook reports a runtime
// failure or Stop is called from elsewhere, and then stops the App with
// context.WithoutCancel(ctx) (or waits for the Stop already in progress). If
// ctx comes from SignalContext and a second signal arrives, the stop context
// is cancelled immediately, abandoning the remaining stop procedure.
//
// Run returns the start error if Start fails; otherwise it returns
// errors.Join(runtime failures, stop error), which is nil after a clean
// shutdown.
func (a *App) Run(ctx context.Context) error {
	if err := a.start(ctx, true); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
	case <-a.fatal:
	case <-a.quit: // an external Stop has begun
	}

	stopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	if abort, ok := ctx.Value(abortKey{}).(context.Context); ok {
		defer context.AfterFunc(abort, cancel)()
	}
	stopErr := a.Stop(stopCtx)

	a.mu.Lock()
	errs := append(slices.Clone(a.failures), stopErr)
	a.mu.Unlock()
	return errors.Join(errs...)
}

// Check reports readiness. Outside StateRunning it returns
// "app: not ready: state=<state>". Otherwise it runs every hook implementing
// Check(ctx) error concurrently and joins failures as "app: hook %q not ready: %w".
// It waits for all checks to return, so they must honor ctx. The lifecycle may
// advance while checks run.
func (a *App) Check(ctx context.Context) error {
	if s := a.State(); s != StateRunning {
		return fmt.Errorf("app: not ready: state=%s", s)
	}
	errs := make([]error, len(a.hooks))
	var wg sync.WaitGroup
	for i, h := range a.hooks {
		c, ok := h.(readinessChecker)
		if !ok {
			continue
		}
		wg.Go(func() {
			if err := c.Check(ctx); err != nil {
				errs[i] = fmt.Errorf("app: hook %q not ready: %w", h.Name(), err)
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (a *App) setState(s State) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = s
}
