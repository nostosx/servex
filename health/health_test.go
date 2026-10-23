package health

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestCheckerFunc(t *testing.T) {
	want := errors.New("down")
	var c Checker = CheckerFunc(func(context.Context) error { return want })
	if err := c.Check(t.Context()); !errors.Is(err, want) {
		t.Fatalf("Check() = %v, want %v", err, want)
	}
}

func TestGroupCheck(t *testing.T) {
	errA := errors.New("a failed")
	errC := errors.New("c failed")
	ok := CheckerFunc(func(context.Context) error { return nil })
	tests := []struct {
		name   string
		checks map[string]Checker
		want   string // "" means nil
		is     []error
	}{
		{name: "empty", checks: nil},
		{name: "all pass", checks: map[string]Checker{"x": ok, "y": ok}},
		{
			name:   "single failure",
			checks: map[string]Checker{"b": ok, "a": CheckerFunc(func(context.Context) error { return errA })},
			want:   "health: a: a failed",
			is:     []error{errA},
		},
		{
			name: "multiple failures sorted by name",
			checks: map[string]Checker{
				"c": CheckerFunc(func(context.Context) error { return errC }),
				"b": ok,
				"a": CheckerFunc(func(context.Context) error { return errA }),
			},
			want: "health: a: a failed\nhealth: c: c failed",
			is:   []error{errA, errC},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGroup()
			for name, c := range tt.checks {
				g.Add(name, c)
			}
			err := g.Check(t.Context())
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Check() = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tt.want {
				t.Fatalf("Check() = %v, want %q", err, tt.want)
			}
			for _, target := range tt.is {
				if !errors.Is(err, target) {
					t.Errorf("errors.Is(err, %v) = false", target)
				}
			}
		})
	}
}

func TestGroupAddOverridesAndRemoves(t *testing.T) {
	g := NewGroup()
	g.Add("db", CheckerFunc(func(context.Context) error { return errors.New("old") }))
	g.Add("db", CheckerFunc(func(context.Context) error { return nil }))
	if err := g.Check(t.Context()); err != nil {
		t.Fatalf("Check() after override = %v, want nil", err)
	}
	if got := len(g.Report(t.Context())); got != 1 {
		t.Fatalf("len(Report()) = %d, want 1", got)
	}
	g.Add("db", nil)
	if got := len(g.Report(t.Context())); got != 0 {
		t.Fatalf("len(Report()) after removal = %d, want 0", got)
	}
}

func TestGroupReportSortedWithDurations(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := NewGroup()
		for _, name := range []string{"zeta", "alpha", "mid"} {
			d := time.Duration(len(name)) * time.Second
			g.Add(name, CheckerFunc(func(ctx context.Context) error {
				time.Sleep(d)
				return nil
			}))
		}
		start := time.Now()
		got := g.Report(t.Context())
		elapsed := time.Since(start)

		wantNames := []string{"alpha", "mid", "zeta"}
		wantDur := []time.Duration{5 * time.Second, 3 * time.Second, 4 * time.Second}
		if len(got) != len(wantNames) {
			t.Fatalf("len(Report()) = %d, want %d", len(got), len(wantNames))
		}
		for i, r := range got {
			if r.Name != wantNames[i] || r.Err != nil || r.Duration != wantDur[i] {
				t.Errorf("Report()[%d] = %+v, want name %q duration %v", i, r, wantNames[i], wantDur[i])
			}
		}
		// Checks run concurrently: total time is the slowest check, not the sum.
		if elapsed != 5*time.Second {
			t.Errorf("Report() took %v, want 5s", elapsed)
		}
	})
}

func TestGroupRunsConcurrently(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const n = 8
		g := NewGroup()
		var running, peak atomic.Int32
		release := make(chan struct{})
		for i := range n {
			g.Add(string(rune('a'+i)), CheckerFunc(func(context.Context) error {
				cur := running.Add(1)
				for {
					p := peak.Load()
					if cur <= p || peak.CompareAndSwap(p, cur) {
						break
					}
				}
				<-release
				running.Add(-1)
				return nil
			}))
		}
		done := make(chan error)
		go func() { done <- g.Check(t.Context()) }()
		synctest.Wait()
		if got := peak.Load(); got != n {
			t.Errorf("peak concurrency = %d, want %d", got, n)
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatalf("Check() = %v", err)
		}
	})
}

func TestGroupPassesContext(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(t.Context(), key{}, "v")
	g := NewGroup()
	g.Add("ctx", CheckerFunc(func(ctx context.Context) error {
		if ctx.Value(key{}) != "v" {
			return errors.New("context not propagated")
		}
		return nil
	}))
	if err := g.Check(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestGroupConcurrentAddAndCheck(t *testing.T) {
	g := NewGroup()
	ok := CheckerFunc(func(context.Context) error { return nil })
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 100 {
			g.Add(string(rune('a'+i%26)), ok)
		}
	}()
	for range 100 {
		if err := g.Check(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	<-done
}

func BenchmarkGroupCheck(b *testing.B) {
	g := NewGroup()
	for i := range 4 {
		g.Add(string(rune('a'+i)), CheckerFunc(func(context.Context) error { return nil }))
	}
	ctx := b.Context()
	for b.Loop() {
		_ = g.Check(ctx)
	}
}
