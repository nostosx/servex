package app

import "context"

// Hook is an entity that takes part in the application lifecycle; it is not
// an event callback.
//
// A Hook may optionally implement:
//
//	Err() <-chan error         reports fatal runtime errors. A non-nil error
//	                           triggers an overall stop that ends in StateFailed.
//	                           A closed channel or a nil error is ignored.
//	Check(ctx) error           takes part in the readiness aggregation of App.Check.
//	Drain(ctx) error           stops taking new work before any hook stops. When
//	                           a running App stops, it drains hooks one at a time
//	                           in reverse order, then waits for the stop delay and
//	                           only then calls Stop. Drain is not called when a
//	                           failed Start is rolled back, so Stop must still
//	                           release everything on its own.
type Hook interface {
	// Name identifies the hook in logs and errors.
	Name() string
	// Start brings the hook up. It must return once the hook is ready.
	Start(ctx context.Context) error
	// Stop shuts the hook down, honouring ctx where possible.
	Stop(ctx context.Context) error
}

// errReporter is the optional runtime failure interface of a Hook.
type errReporter interface {
	Err() <-chan error
}

// readinessChecker is the optional readiness interface of a Hook.
type readinessChecker interface {
	Check(ctx context.Context) error
}

// drainer is the optional drain interface of a Hook.
type drainer interface {
	Drain(ctx context.Context) error
}

// NewHook builds a Hook from functions. start and stop may be nil, in which
// case the corresponding method does nothing and returns nil.
func NewHook(name string, start, stop func(ctx context.Context) error) Hook {
	return &funcHook{name: name, start: start, stop: stop}
}

type funcHook struct {
	name        string
	start, stop func(ctx context.Context) error
}

func (h *funcHook) Name() string { return h.name }

func (h *funcHook) Start(ctx context.Context) error {
	if h.start == nil {
		return nil
	}
	return h.start(ctx)
}

func (h *funcHook) Stop(ctx context.Context) error {
	if h.stop == nil {
		return nil
	}
	return h.stop(ctx)
}
