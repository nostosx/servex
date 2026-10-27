package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// drainHook is a Hook that also drains.
type drainHook struct {
	Hook
	drain func(ctx context.Context) error
}

func (h *drainHook) Drain(ctx context.Context) error { return h.drain(ctx) }

func recDrainHook(r *recorder, name string, drainErr error) Hook {
	return &drainHook{Hook: recHook(r, name, nil, nil), drain: func(context.Context) error {
		r.add("drain:" + name)
		return drainErr
	}}
}

func TestDrainBeforeStopDelayAndStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &recorder{}
		var begin time.Time
		var drainedAt, stoppedAt time.Duration
		probe := &drainHook{
			Hook: NewHook("probe", nil, func(context.Context) error {
				stoppedAt = time.Since(begin)
				return nil
			}),
			drain: func(context.Context) error {
				drainedAt = time.Since(begin)
				return nil
			},
		}
		a := New(quiet(), WithStopDelay(time.Second), WithHooks(
			recDrainHook(r, "a", nil), recHook(r, "b", nil, nil), recDrainHook(r, "c", nil), probe,
		))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		begin = time.Now()
		if err := a.Stop(t.Context()); err != nil {
			t.Fatalf("Stop() = %v", err)
		}
		if drainedAt != 0 || stoppedAt != time.Second {
			t.Errorf("drained at %v and stopped at %v, want 0 and the 1s stop delay", drainedAt, stoppedAt)
		}
		want := []string{"start:a", "start:b", "start:c", "drain:c", "drain:a", "stop:c", "stop:b", "stop:a"}
		if got := r.get(); !slices.Equal(got, want) {
			t.Errorf("events = %v, want %v", got, want)
		}
	})
}

func TestDrainErrorsDoNotAbortStop(t *testing.T) {
	r := &recorder{}
	errDrain := errors.New("drain failed")
	a := New(quiet(), WithHooks(recDrainHook(r, "a", nil), recDrainHook(r, "b", errDrain)))
	if err := a.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	err := a.Stop(t.Context())
	if !errors.Is(err, errDrain) || !strings.Contains(err.Error(), `app: drain hook "b"`) {
		t.Fatalf("Stop() = %v, want the drain error of b", err)
	}
	if want := []string{"start:a", "start:b", "drain:b", "drain:a", "stop:b", "stop:a"}; !slices.Equal(r.get(), want) {
		t.Errorf("events = %v, want %v", r.get(), want)
	}
	if s := a.State(); s != StateFailed {
		t.Errorf("state = %v, want failed", s)
	}
}

func TestDrainHappensWhileStopping(t *testing.T) {
	var state State
	var a *App
	a = New(quiet(), WithHooks(&drainHook{Hook: NewHook("d", nil, nil), drain: func(ctx context.Context) error {
		state = a.State()
		return a.Check(ctx)
	}}))
	if err := a.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	err := a.Stop(t.Context())
	if state != StateStopping || err == nil {
		t.Errorf("drain saw state %v and Stop = %v; want stopping and a readiness failure", state, err)
	}
}

func TestNoDrainOnStartRollback(t *testing.T) {
	r := &recorder{}
	errStart := errors.New("boom")
	a := New(quiet(), WithHooks(recDrainHook(r, "a", nil), recHook(r, "b", errStart, nil)))
	if err := a.Start(t.Context()); !errors.Is(err, errStart) {
		t.Fatalf("Start() = %v", err)
	}
	if want := []string{"start:a", "start:b", "stop:a"}; !slices.Equal(r.get(), want) {
		t.Errorf("events = %v, want %v", r.get(), want)
	}
	// Stop after a failed start does nothing.
	if err := a.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n := len(r.get()); n != 3 {
		t.Errorf("events after Stop = %v", r.get())
	}
}

func TestDrainBoundedByStopTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &recorder{}
		release := make(chan struct{})
		stuck := &drainHook{Hook: recHook(r, "stuck", nil, nil), drain: func(context.Context) error {
			<-release // ignores ctx
			return nil
		}}
		a := New(quiet(), WithStopTimeout(time.Second), WithHooks(recDrainHook(r, "a", nil), stuck))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		err := a.Stop(t.Context())
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), `app: drain hook "stuck"`) ||
			!strings.Contains(err.Error(), `app: drain hook "a"`) || !strings.Contains(err.Error(), "stop timeout") {
			t.Errorf("Stop() = %v", err)
		}
		if want := []string{"start:a", "start:stuck"}; !slices.Equal(r.get(), want) {
			t.Errorf("events = %v, want %v", r.get(), want)
		}
		close(release)
	})
}
