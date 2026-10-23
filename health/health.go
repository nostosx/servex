package health

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"
)

// Checker reports whether a component is healthy. A nil error means healthy.
type Checker interface {
	Check(ctx context.Context) error
}

// CheckerFunc adapts an ordinary function to the Checker interface.
type CheckerFunc func(ctx context.Context) error

// Check calls f(ctx).
func (f CheckerFunc) Check(ctx context.Context) error { return f(ctx) }

// Result is the outcome of a single named check.
type Result struct {
	// Name is the name the check was registered under.
	Name string
	// Err is the error returned by the check, nil when it passed.
	Err error
	// Duration is how long the check took.
	Duration time.Duration
}

// Group runs a set of named checks concurrently. The zero value is not
// usable; create groups with NewGroup. Group is safe for concurrent use;
// registered checkers must also support concurrent calls.
type Group struct {
	mu     sync.RWMutex
	checks map[string]Checker
}

// NewGroup returns an empty Group.
func NewGroup() *Group {
	return &Group{checks: make(map[string]Checker)}
}

// Add registers c under name, replacing any check already registered under
// the same name. A nil c removes the named check.
func (g *Group) Add(name string, c Checker) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c == nil {
		delete(g.checks, name)
		return
	}
	g.checks[name] = c
}

// Check runs every registered check concurrently. It returns nil when all of
// them pass; otherwise it returns the errors.Join of every failure, each
// formatted as "health: <name>: <err>" in ascending name order.
func (g *Group) Check(ctx context.Context) error {
	var errs []error
	for _, r := range g.Report(ctx) {
		if r.Err != nil {
			errs = append(errs, fmt.Errorf("health: %s: %w", r.Name, r.Err))
		}
	}
	return errors.Join(errs...)
}

// Report runs every registered check concurrently and returns one Result per
// check, sorted by name in ascending order. It snapshots the registrations
// before starting checks and waits for every check to return. Checkers must
// honor ctx for cancellation to finish promptly.
func (g *Group) Report(ctx context.Context) []Result {
	g.mu.RLock()
	names := slices.Sorted(maps.Keys(g.checks))
	checks := make([]Checker, len(names))
	for i, name := range names {
		checks[i] = g.checks[name]
	}
	g.mu.RUnlock()

	results := make([]Result, len(names))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Go(func() {
			begin := time.Now()
			err := c.Check(ctx)
			results[i] = Result{Name: names[i], Err: err, Duration: time.Since(begin)}
		})
	}
	wg.Wait()
	return results
}
