package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// App satisfies health.Checker structurally; app must not import health.
var _ interface {
	Check(ctx context.Context) error
} = (*App)(nil)

// recorder collects lifecycle events in order.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(e string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

// recHook returns a Hook that records "start:<name>" / "stop:<name>" and
// returns the given errors.
func recHook(r *recorder, name string, startErr, stopErr error) Hook {
	return NewHook(name,
		func(context.Context) error { r.add("start:" + name); return startErr },
		func(context.Context) error { r.add("stop:" + name); return stopErr },
	)
}

// errHook is a Hook that also reports runtime errors.
type errHook struct {
	Hook
	ch chan error
}

func (h *errHook) Err() <-chan error { return h.ch }

// checkHook is a Hook that also takes part in readiness.
type checkHook struct {
	Hook
	check func(ctx context.Context) error
}

func (h *checkHook) Check(ctx context.Context) error { return h.check(ctx) }

func quiet() Option { return WithLogger(testLogger(slog.New(slog.DiscardHandler))) }

func TestStateString(t *testing.T) {
	tests := []struct {
		s    State
		want string
	}{
		{StateInit, "init"},
		{StateStarting, "starting"},
		{StateRunning, "running"},
		{StateStopping, "stopping"},
		{StateStopped, "stopped"},
		{StateFailed, "failed"},
		{State(99), "State(99)"},
		{State(-1), "State(-1)"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", int(tt.s), got, tt.want)
		}
	}
}

func TestNewHookNilFuncs(t *testing.T) {
	h := NewHook("noop", nil, nil)
	if h.Name() != "noop" {
		t.Fatalf("Name() = %q", h.Name())
	}
	if err := h.Start(t.Context()); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	if err := h.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
}

func TestStartStopOrder(t *testing.T) {
	r := &recorder{}
	a := New(quiet(), WithHooks(recHook(r, "a", nil, nil), nil, recHook(r, "b", nil, nil)), WithHooks(recHook(r, "c", nil, nil)))
	if s := a.State(); s != StateInit {
		t.Fatalf("initial state = %v", s)
	}
	if err := a.Start(t.Context()); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	if s := a.State(); s != StateRunning {
		t.Fatalf("state after Start = %v, want running", s)
	}
	if err := a.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	if s := a.State(); s != StateStopped {
		t.Fatalf("state after Stop = %v, want stopped", s)
	}
	want := []string{"start:a", "start:b", "start:c", "stop:c", "stop:b", "stop:a"}
	if got := r.get(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestStartRollback(t *testing.T) {
	errStart := errors.New("start boom")
	errStopA := errors.New("stop boom")
	tests := []struct {
		name       string
		hooks      func(r *recorder) []Hook
		wantEvents []string
		wantErr    string
		wantIs     []error
	}{
		{
			name: "first hook fails",
			hooks: func(r *recorder) []Hook {
				return []Hook{recHook(r, "a", errStart, nil), recHook(r, "b", nil, nil)}
			},
			wantEvents: []string{"start:a"},
			wantErr:    `app: start hook "a": start boom`,
			wantIs:     []error{errStart},
		},
		{
			name: "middle hook fails, rollback clean",
			hooks: func(r *recorder) []Hook {
				return []Hook{recHook(r, "a", nil, nil), recHook(r, "b", nil, nil), recHook(r, "c", errStart, nil), recHook(r, "d", nil, nil)}
			},
			wantEvents: []string{"start:a", "start:b", "start:c", "stop:b", "stop:a"},
			wantErr:    `app: start hook "c": start boom`,
			wantIs:     []error{errStart},
		},
		{
			name: "rollback stop fails",
			hooks: func(r *recorder) []Hook {
				return []Hook{recHook(r, "a", nil, errStopA), recHook(r, "b", nil, nil), recHook(r, "c", errStart, nil)}
			},
			wantEvents: []string{"start:a", "start:b", "start:c", "stop:b", "stop:a"},
			wantErr:    "app: start hook \"c\": start boom\napp: stop hook \"a\": stop boom",
			wantIs:     []error{errStart, errStopA},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{}
			a := New(quiet(), WithHooks(tt.hooks(r)...))
			err := a.Start(t.Context())
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("Start() = %v, want %q", err, tt.wantErr)
			}
			for _, target := range tt.wantIs {
				if !errors.Is(err, target) {
					t.Errorf("errors.Is(err, %v) = false", target)
				}
			}
			if got := r.get(); !slices.Equal(got, tt.wantEvents) {
				t.Errorf("events = %v, want %v", got, tt.wantEvents)
			}
			if s := a.State(); s != StateFailed {
				t.Errorf("state = %v, want failed", s)
			}
			// Failed is terminal.
			if err := a.Start(t.Context()); !errors.Is(err, ErrInvalidState) {
				t.Errorf("second Start() = %v, want ErrInvalidState", err)
			}
			if err := a.Stop(t.Context()); err != nil {
				t.Errorf("Stop() after failed start = %v, want nil", err)
			}
			if s := a.State(); s != StateFailed {
				t.Errorf("state after Stop = %v, want failed", s)
			}
		})
	}
}

func TestStartHonoursCanceledContext(t *testing.T) {
	r := &recorder{}
	a := New(quiet(), WithHooks(recHook(r, "a", nil, nil)))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := a.Start(ctx)
	if !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), `app: start hook "a": `) {
		t.Fatalf("Start() = %v, want wrapped context.Canceled", err)
	}
	if got := r.get(); len(got) != 0 {
		t.Fatalf("events = %v, want none", got)
	}
}

func TestRollbackBoundedByStopTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		defer close(release)
		block := NewHook("block", nil, func(context.Context) error { <-release; return nil })
		fail := NewHook("fail", func(context.Context) error { return errors.New("x") }, nil)
		a := New(quiet(), WithStopTimeout(time.Second), WithHooks(block, fail))
		begin := time.Now()
		err := a.Start(t.Context())
		want := "app: start hook \"fail\": x\napp: stop timeout, unfinished hooks: [block]"
		if err == nil || err.Error() != want {
			t.Fatalf("Start() = %v, want %q", err, want)
		}
		if d := time.Since(begin); d != time.Second {
			t.Fatalf("Start() took %v, want 1s", d)
		}
	})
}

func TestInvalidState(t *testing.T) {
	a := New(quiet())
	if err := a.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	err := a.Start(t.Context())
	if !errors.Is(err, ErrInvalidState) || err.Error() != "app: invalid state: running" {
		t.Fatalf("second Start() = %v, want %q", err, "app: invalid state: running")
	}
	if err := a.Run(t.Context()); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Run() on running app = %v, want ErrInvalidState", err)
	}
	if err := a.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := a.Start(t.Context()); !errors.Is(err, ErrInvalidState) || err.Error() != "app: invalid state: stopped" {
		t.Fatalf("Start() after Stop = %v", err)
	}
}

func TestStopFromInit(t *testing.T) {
	r := &recorder{}
	a := New(quiet(), WithHooks(recHook(r, "a", nil, nil)))
	if err := a.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() = %v, want nil", err)
	}
	if s := a.State(); s != StateStopped {
		t.Fatalf("state = %v, want stopped", s)
	}
	if err := a.Start(t.Context()); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Start() after Stop = %v, want ErrInvalidState", err)
	}
	if got := r.get(); len(got) != 0 {
		t.Fatalf("events = %v, want none", got)
	}
}

func TestStopHookErrors(t *testing.T) {
	r := &recorder{}
	errB := errors.New("b broke")
	a := New(quiet(), WithHooks(recHook(r, "a", nil, nil), recHook(r, "b", nil, errB), recHook(r, "c", nil, nil)))
	if err := a.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	err := a.Stop(t.Context())
	if !errors.Is(err, errB) || err.Error() != `app: stop hook "b": b broke` {
		t.Fatalf("Stop() = %v", err)
	}
	want := []string{"start:a", "start:b", "start:c", "stop:c", "stop:b", "stop:a"}
	if got := r.get(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v (stop continues after a failure)", got, want)
	}
	if s := a.State(); s != StateFailed {
		t.Fatalf("state = %v, want failed", s)
	}
	if again := a.Stop(t.Context()); again != err {
		t.Fatalf("repeated Stop() = %v, want first result %v", again, err)
	}
}

func TestStopIdempotentConcurrent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		errStop := errors.New("stop failed")
		h := NewHook("h", nil, func(context.Context) error {
			calls.Add(1)
			time.Sleep(time.Second)
			return errStop
		})
		a := New(quiet(), WithHooks(h))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		const n = 16
		results := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() { results[i] = a.Stop(context.Background()) })
		}
		wg.Wait()
		if got := calls.Load(); got != 1 {
			t.Fatalf("hook Stop called %d times, want 1", got)
		}
		for i, err := range results {
			if err != results[0] || !errors.Is(err, errStop) {
				t.Errorf("results[%d] = %v, want first result %v", i, err, results[0])
			}
		}
		if again := a.Stop(t.Context()); again != results[0] {
			t.Errorf("later Stop() = %v, want %v", again, results[0])
		}
	})
}

func TestStopWaiterContextExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		h := NewHook("slow", nil, func(context.Context) error { <-release; return nil })
		a := New(quiet(), WithStopTimeout(0), WithHooks(h))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		first := make(chan error)
		go func() { first <- a.Stop(context.Background()) }()
		synctest.Wait()

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if err := a.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("waiting Stop() = %v, want DeadlineExceeded", err)
		}
		close(release)
		if err := <-first; err != nil {
			t.Fatalf("first Stop() = %v", err)
		}
	})
}

func TestStopWaitsForStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &recorder{}
		release := make(chan struct{})
		slow := NewHook("slow",
			func(context.Context) error { <-release; r.add("start:slow"); return nil },
			func(context.Context) error { r.add("stop:slow"); return nil },
		)
		a := New(quiet(), WithHooks(slow))
		startErr := make(chan error)
		go func() { startErr <- a.Start(t.Context()) }()
		synctest.Wait()
		if s := a.State(); s != StateStarting {
			t.Fatalf("state = %v, want starting", s)
		}
		if err := a.Check(t.Context()); err == nil || err.Error() != "app: not ready: state=starting" {
			t.Fatalf("Check() = %v", err)
		}
		stopErr := make(chan error)
		go func() { stopErr <- a.Stop(t.Context()) }()
		synctest.Wait()
		close(release)
		if err := <-startErr; err != nil {
			t.Fatalf("Start() = %v", err)
		}
		if err := <-stopErr; err != nil {
			t.Fatalf("Stop() = %v", err)
		}
		if got, want := r.get(), []string{"start:slow", "stop:slow"}; !slices.Equal(got, want) {
			t.Fatalf("events = %v, want %v", got, want)
		}
		if s := a.State(); s != StateStopped {
			t.Fatalf("state = %v, want stopped", s)
		}
	})
}

func TestStopDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = 5 * time.Second
		r := &recorder{}
		h := &checkHook{Hook: recHook(r, "h", nil, nil), check: func(context.Context) error { return nil }}
		a := New(quiet(), WithStopDelay(delay), WithHooks(h))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := a.Check(t.Context()); err != nil {
			t.Fatalf("Check() while running = %v", err)
		}
		done := make(chan error)
		go func() { done <- a.Stop(t.Context()) }()
		synctest.Wait()

		// Readiness fails as soon as Stopping is entered, before the delay
		// elapses and before any hook is stopped.
		if s := a.State(); s != StateStopping {
			t.Fatalf("state = %v, want stopping", s)
		}
		if err := a.Check(t.Context()); err == nil || err.Error() != "app: not ready: state=stopping" {
			t.Fatalf("Check() while stopping = %v", err)
		}
		if got := r.get(); slices.Contains(got, "stop:h") {
			t.Fatalf("hook stopped before delay: %v", got)
		}

		synctest.Sleep(delay - time.Nanosecond)
		if got := r.get(); slices.Contains(got, "stop:h") {
			t.Fatalf("hook stopped before delay elapsed: %v", got)
		}
		synctest.Sleep(time.Nanosecond)
		if got := r.get(); !slices.Contains(got, "stop:h") {
			t.Fatalf("hook not stopped after delay: %v", got)
		}
		if err := <-done; err != nil {
			t.Fatalf("Stop() = %v", err)
		}
	})
}

func TestStopDelayBoundedByContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &recorder{}
		a := New(quiet(), WithStopDelay(time.Hour), WithHooks(recHook(r, "a", nil, nil), recHook(r, "b", nil, nil)))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		begin := time.Now()
		err := a.Stop(ctx)
		if err == nil || err.Error() != "app: stop timeout, unfinished hooks: [b a]" {
			t.Fatalf("Stop() = %v", err)
		}
		if d := time.Since(begin); d != 2*time.Second {
			t.Fatalf("Stop() took %v, want 2s", d)
		}
		if got := r.get(); slices.ContainsFunc(got, func(e string) bool { return strings.HasPrefix(e, "stop:") }) {
			t.Fatalf("hooks stopped after timeout: %v", got)
		}
		if s := a.State(); s != StateFailed {
			t.Fatalf("state = %v, want failed", s)
		}
	})
}

func TestStopTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &recorder{}
		release := make(chan struct{})
		// The hook ignores ctx and blocks until the test releases it.
		stuck := NewHook("stuck", nil, func(context.Context) error {
			r.add("stop:stuck")
			<-release
			return nil
		})
		a := New(quiet(), WithStopTimeout(3*time.Second),
			WithHooks(recHook(r, "first", nil, nil), recHook(r, "second", nil, nil), stuck, recHook(r, "last", nil, nil)))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		begin := time.Now()
		err := a.Stop(context.Background())
		elapsed := time.Since(begin)
		close(release)

		want := "app: stop timeout, unfinished hooks: [stuck second first]"
		if err == nil || err.Error() != want {
			t.Fatalf("Stop() = %v, want %q", err, want)
		}
		if elapsed != 3*time.Second {
			t.Fatalf("Stop() took %v, want 3s", elapsed)
		}
		wantEvents := []string{"start:first", "start:second", "start:last", "stop:last", "stop:stuck"}
		if got := r.get(); !slices.Equal(got, wantEvents) {
			t.Fatalf("events = %v, want %v", got, wantEvents)
		}
		if s := a.State(); s != StateFailed {
			t.Fatalf("state = %v, want failed", s)
		}
	})
}

func TestStopTimeoutUsesEarlierContextDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		defer close(release)
		stuck := NewHook("stuck", nil, func(context.Context) error { <-release; return nil })
		a := New(quiet(), WithStopTimeout(time.Minute), WithHooks(stuck))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		begin := time.Now()
		if err := a.Stop(ctx); err == nil || err.Error() != "app: stop timeout, unfinished hooks: [stuck]" {
			t.Fatalf("Stop() = %v", err)
		}
		if d := time.Since(begin); d != time.Second {
			t.Fatalf("Stop() took %v, want 1s", d)
		}
	})
}

func TestStopTimeoutUnlimited(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		synctest.Test(t, func(t *testing.T) {
			slow := NewHook("slow", nil, func(context.Context) error { time.Sleep(time.Hour); return nil })
			a := New(quiet(), WithStopTimeout(timeout), WithHooks(slow))
			if err := a.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := a.Stop(context.Background()); err != nil {
				t.Fatalf("Stop() with timeout %v = %v, want nil", timeout, err)
			}
			if s := a.State(); s != StateStopped {
				t.Fatalf("state = %v, want stopped", s)
			}
		})
	}
}

func TestDefaultStopTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		defer close(release)
		stuck := NewHook("stuck", nil, func(context.Context) error { <-release; return nil })
		a := New(quiet(), WithHooks(stuck))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		begin := time.Now()
		if err := a.Stop(context.Background()); err == nil {
			t.Fatal("Stop() = nil, want timeout")
		}
		if d := time.Since(begin); d != 30*time.Second {
			t.Fatalf("Stop() took %v, want 30s", d)
		}
	})
}

func TestCheck(t *testing.T) {
	errDB := errors.New("db down")
	errMQ := errors.New("mq down")
	ok := func(context.Context) error { return nil }
	tests := []struct {
		name    string
		hooks   []Hook
		wantErr string // "" means nil
		wantIs  []error
	}{
		{name: "no checkers", hooks: []Hook{NewHook("plain", nil, nil)}},
		{name: "all ready", hooks: []Hook{
			&checkHook{Hook: NewHook("a", nil, nil), check: ok},
			&checkHook{Hook: NewHook("b", nil, nil), check: ok},
		}},
		{
			name: "failures joined in registration order",
			hooks: []Hook{
				&checkHook{Hook: NewHook("db", nil, nil), check: func(context.Context) error { return errDB }},
				&checkHook{Hook: NewHook("ok", nil, nil), check: ok},
				NewHook("plain", nil, nil),
				&checkHook{Hook: NewHook("mq", nil, nil), check: func(context.Context) error { return errMQ }},
			},
			wantErr: "app: hook \"db\" not ready: db down\napp: hook \"mq\" not ready: mq down",
			wantIs:  []error{errDB, errMQ},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(quiet(), WithHooks(tt.hooks...))
			if err := a.Check(t.Context()); err == nil || err.Error() != "app: not ready: state=init" {
				t.Fatalf("Check() before Start = %v", err)
			}
			if err := a.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			err := a.Check(t.Context())
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("Check() = %v, want nil", err)
			case tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr):
				t.Fatalf("Check() = %v, want %q", err, tt.wantErr)
			}
			for _, target := range tt.wantIs {
				if !errors.Is(err, target) {
					t.Errorf("errors.Is(err, %v) = false", target)
				}
			}
			if err := a.Stop(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := a.Check(t.Context()); err == nil || err.Error() != "app: not ready: state=stopped" {
				t.Fatalf("Check() after Stop = %v", err)
			}
		})
	}
}

func TestCheckFailedState(t *testing.T) {
	a := New(quiet(), WithHooks(NewHook("x", func(context.Context) error { return errors.New("no") }, nil)))
	_ = a.Start(t.Context())
	if err := a.Check(t.Context()); err == nil || err.Error() != "app: not ready: state=failed" {
		t.Fatalf("Check() = %v", err)
	}
}

func TestErrTriggersStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &recorder{}
		boom := errors.New("listener died")
		eh := &errHook{Hook: recHook(r, "server", nil, nil), ch: make(chan error, 1)}
		a := New(quiet(), WithHooks(recHook(r, "db", nil, nil), eh))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		eh.ch <- boom
		synctest.Wait()
		if s := a.State(); s != StateFailed {
			t.Fatalf("state after runtime error = %v, want failed", s)
		}
		want := []string{"start:db", "start:server", "stop:server", "stop:db"}
		if got := r.get(); !slices.Equal(got, want) {
			t.Fatalf("events = %v, want %v", got, want)
		}
		// Stop errors only cover stopping; the runtime failure is reflected in
		// the Failed state (and in Run's result).
		if err := a.Stop(t.Context()); err != nil {
			t.Fatalf("Stop() = %v, want nil", err)
		}
	})
}

func TestErrIgnoresNilAndClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		nilSender := &errHook{Hook: NewHook("nil", nil, nil), ch: make(chan error, 1)}
		closed := &errHook{Hook: NewHook("closed", nil, nil), ch: make(chan error)}
		noChan := &errHook{Hook: NewHook("nochan", nil, nil)} // nil channel
		a := New(quiet(), WithHooks(nilSender, closed, noChan))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		nilSender.ch <- nil
		close(closed.ch)
		synctest.Wait()
		if s := a.State(); s != StateRunning {
			t.Fatalf("state = %v, want running", s)
		}
		if err := a.Check(t.Context()); err != nil {
			t.Fatalf("Check() = %v", err)
		}
		if err := a.Stop(t.Context()); err != nil {
			t.Fatalf("Stop() = %v", err)
		}
		if s := a.State(); s != StateStopped {
			t.Fatalf("state = %v, want stopped", s)
		}
	})
}

func TestRunCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &recorder{}
		a := New(quiet(), WithHooks(recHook(r, "a", nil, nil), recHook(r, "b", nil, nil)))
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error)
		go func() { done <- a.Run(ctx) }()
		synctest.Wait()
		if s := a.State(); s != StateRunning {
			t.Fatalf("state = %v, want running", s)
		}
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
		want := []string{"start:a", "start:b", "stop:b", "stop:a"}
		if got := r.get(); !slices.Equal(got, want) {
			t.Fatalf("events = %v, want %v", got, want)
		}
		if s := a.State(); s != StateStopped {
			t.Fatalf("state = %v, want stopped", s)
		}
	})
}

func TestRunReturnsAfterExternalStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &recorder{}
		a := New(quiet(), WithHooks(recHook(r, "a", nil, nil)))
		done := make(chan error)
		go func() { done <- a.Run(t.Context()) }() // ctx never ends on its own
		synctest.Wait()
		if err := a.Stop(t.Context()); err != nil {
			t.Fatalf("Stop() = %v", err)
		}
		if err := <-done; err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
		want := []string{"start:a", "stop:a"}
		if got := r.get(); !slices.Equal(got, want) {
			t.Fatalf("events = %v, want %v (hooks must stop exactly once)", got, want)
		}
	})
}

func TestRunStopUsesUncanceledContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var stopCtxErr error
		h := NewHook("h", nil, func(ctx context.Context) error { stopCtxErr = ctx.Err(); return nil })
		a := New(quiet(), WithHooks(h))
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error)
		go func() { done <- a.Run(ctx) }()
		synctest.Wait()
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if stopCtxErr != nil {
			t.Fatalf("hook Stop saw ctx.Err() = %v, want nil", stopCtxErr)
		}
	})
}

func TestRunRuntimeFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		boom := errors.New("serve failed")
		errStop := errors.New("close failed")
		eh := &errHook{Hook: NewHook("server", nil, nil), ch: make(chan error, 1)}
		db := NewHook("db", nil, func(context.Context) error { return errStop })
		a := New(quiet(), WithHooks(db, eh))
		done := make(chan error)
		go func() { done <- a.Run(t.Context()) }()
		synctest.Wait()
		eh.ch <- boom
		err := <-done
		want := "app: hook \"server\": serve failed\napp: stop hook \"db\": close failed"
		if err == nil || err.Error() != want {
			t.Fatalf("Run() = %v, want %q", err, want)
		}
		if !errors.Is(err, boom) || !errors.Is(err, errStop) {
			t.Fatalf("Run() error does not wrap both causes: %v", err)
		}
		if s := a.State(); s != StateFailed {
			t.Fatalf("state = %v, want failed", s)
		}
	})
}

func TestRunStartFailure(t *testing.T) {
	boom := errors.New("boom")
	a := New(quiet(), WithHooks(NewHook("x", func(context.Context) error { return boom }, nil)))
	err := a.Run(t.Context())
	if !errors.Is(err, boom) || err.Error() != `app: start hook "x": boom` {
		t.Fatalf("Run() = %v", err)
	}
	if s := a.State(); s != StateFailed {
		t.Fatalf("state = %v, want failed", s)
	}
}

func TestRunAbortFromContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		defer close(release)
		stopping := make(chan struct{})
		stuck := NewHook("stuck", nil, func(context.Context) error { close(stopping); <-release; return nil })
		a := New(quiet(), WithStopTimeout(0), WithHooks(NewHook("other", nil, nil), stuck))

		abortCtx, abort := context.WithCancel(context.Background())
		ctx, cancel := context.WithCancel(t.Context())
		ctx = context.WithValue(ctx, abortKey{}, abortCtx)
		done := make(chan error)
		go func() { done <- a.Run(ctx) }()
		synctest.Wait()
		cancel()
		<-stopping
		synctest.Sleep(time.Hour) // without an abort the stop would wait forever
		select {
		case err := <-done:
			t.Fatalf("Run() returned before abort: %v", err)
		default:
		}
		abort()
		err := <-done
		if err == nil || err.Error() != "app: stop timeout, unfinished hooks: [stuck other]" {
			t.Fatalf("Run() = %v", err)
		}
	})
}

func TestConcurrentStateAndCheck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &checkHook{Hook: NewHook("h", nil, nil), check: func(context.Context) error { return nil }}
		a := New(quiet(), WithStopDelay(time.Second), WithHooks(h))
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for range 50 {
					_ = a.State()
					_ = a.Check(t.Context())
					time.Sleep(50 * time.Millisecond)
				}
			})
		}
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
		if err := a.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
	})
}

func TestLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	a := New(WithLogger(logger), WithHooks(
		NewHook("good", nil, nil),
		NewHook("bad", nil, func(context.Context) error { return errors.New("nope") }),
	))
	if err := a.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = a.Stop(t.Context())
	out := buf.String()
	for _, want := range []string{
		`level=INFO msg="hook started" hook=good duration=`,
		`level=INFO msg="hook started" hook=bad duration=`,
		`level=ERROR msg="hook stop failed" hook=bad duration=`,
		`level=INFO msg="hook stopped" hook=good duration=`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q:\n%s", want, out)
		}
	}
}

func TestWithLoggerNilDisablesLogging(t *testing.T) {
	a := New(WithLogger(nil), nil)
	if a.logger != nil {
		t.Fatal("nil logger selected an implicit backend")
	}
}

func BenchmarkCheck(b *testing.B) {
	hooks := make([]Hook, 4)
	for i := range hooks {
		hooks[i] = &checkHook{Hook: NewHook("h", nil, nil), check: func(context.Context) error { return nil }}
	}
	a := New(quiet(), WithHooks(hooks...))
	if err := a.Start(b.Context()); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = a.Stop(context.Background()) })
	for b.Loop() {
		_ = a.Check(b.Context())
	}
}
