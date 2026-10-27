package discovery

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nostosx/servex/app"
)

// countingRegistrar fails the first failures Deregister calls.
type countingRegistrar struct {
	mu          sync.Mutex
	failures    int
	deregisters int
	active      map[string]bool
	// onDeregister, when set, is called for every Deregister.
	onDeregister func()
}

func (r *countingRegistrar) Register(_ context.Context, ins *Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active == nil {
		r.active = make(map[string]bool)
	}
	r.active[ins.ID] = true
	return nil
}

var errDeregister = errors.New("deregister failed")

func (r *countingRegistrar) Deregister(_ context.Context, ins *Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deregisters++
	if r.onDeregister != nil {
		r.onDeregister()
	}
	if r.failures > 0 {
		r.failures--
		return errDeregister
	}
	delete(r.active, ins.ID)
	return nil
}

func (r *countingRegistrar) state() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.deregisters, len(r.active)
}

// Draining deregisters the instance when the App starts stopping, before
// the stop delay during which servers keep serving in-flight requests.
func TestRegistrationDeregistersOnDrain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &countingRegistrar{}
		srv := server(t, "http://10.0.0.1:8000")
		var begin time.Time
		var deregisteredAt time.Duration = -1
		r.onDeregister = func() { deregisteredAt = time.Since(begin) }
		reg := Registration(r, Instance{ID: "a", Name: "svc"}, srv)
		a := app.New(app.WithStopDelay(time.Second), app.WithLogger(slog.New(slog.DiscardHandler)),
			app.WithHooks(app.NewHook("server", nil, nil), reg))
		if err := a.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		begin = time.Now()
		if err := a.Stop(t.Context()); err != nil {
			t.Fatalf("Stop() = %v", err)
		}
		if n, active := r.state(); n != 1 || active != 0 {
			t.Errorf("deregisters = %d, active = %d; want 1, 0", n, active)
		}
		if deregisteredAt != 0 {
			t.Errorf("deregistered %v after Stop began, want before the 1s stop delay", deregisteredAt)
		}
	})
}

func TestRegistrationStopRetriesFailedDrain(t *testing.T) {
	r := &countingRegistrar{failures: 1}
	h := Registration(r, Instance{ID: "a", Name: "svc"}, server(t, "http://10.0.0.1:8000"))
	d := h.(interface{ Drain(context.Context) error })
	if err := h.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := d.Drain(t.Context()); !errors.Is(err, errDeregister) {
		t.Fatalf("Drain() = %v, want %v", err, errDeregister)
	}
	if err := h.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() = %v, want a successful retry", err)
	}
	if n, active := r.state(); n != 2 || active != 0 {
		t.Errorf("deregisters = %d, active = %d; want 2, 0", n, active)
	}
	if err := h.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.state(); n != 2 {
		t.Errorf("deregisters after a second Stop = %d, want 2", n)
	}
}

func TestRegistrationStopRetriesOnlyOnce(t *testing.T) {
	r := &countingRegistrar{failures: 3}
	h := Registration(r, Instance{ID: "a", Name: "svc"}, server(t, "http://10.0.0.1:8000"))
	d := h.(interface{ Drain(context.Context) error })
	if err := h.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := d.Drain(t.Context()); !errors.Is(err, errDeregister) {
		t.Fatalf("Drain() = %v", err)
	}
	if err := h.Stop(t.Context()); !errors.Is(err, errDeregister) {
		t.Fatalf("Stop() = %v, want the retry error", err)
	}
	if err := h.Stop(t.Context()); err != nil {
		t.Fatalf("second Stop() = %v, want nil: the instance is no longer tracked", err)
	}
	if n, _ := r.state(); n != 2 {
		t.Errorf("deregisters = %d, want 2", n)
	}
}

func TestRegistrationDrainAfterDrainIsNoop(t *testing.T) {
	r := &countingRegistrar{}
	h := Registration(r, Instance{ID: "a", Name: "svc"}, server(t, "http://10.0.0.1:8000"))
	d := h.(interface{ Drain(context.Context) error })
	if err := d.Drain(t.Context()); err != nil {
		t.Fatalf("Drain before Start = %v", err)
	}
	if err := h.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := d.Drain(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.state(); n != 1 {
		t.Errorf("deregisters = %d, want 1", n)
	}
}

func TestRegistrationCopiesWeight(t *testing.T) {
	f := &fakeRegistrar{}
	ins := Instance{Name: "svc", Weight: 4}
	h := Registration(f, ins, server(t, "http://10.0.0.1:8000"))
	if err := h.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Stop(t.Context()) }()
	if len(f.registered) != 1 || f.registered[0].Weight != 4 {
		t.Errorf("registered = %+v, want Weight 4", f.registered)
	}
}
