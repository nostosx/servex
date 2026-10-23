package metrics_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/nostosx/servex/observability/metrics"
)

func TestDefaultBuckets(t *testing.T) {
	want := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	if !slices.Equal(metrics.DefaultBuckets, want) {
		t.Fatalf("DefaultBuckets = %v, want %v", metrics.DefaultBuckets, want)
	}
	if !slices.IsSorted(metrics.DefaultBuckets) {
		t.Fatal("DefaultBuckets must be ascending")
	}
}

func TestNoopUsable(t *testing.T) {
	m := metrics.Noop()
	if m == nil {
		t.Fatal("Noop returned nil")
	}
	c := m.Counter("requests_total", "help", "method")
	g := m.Gauge("in_flight", "help")
	h := m.Histogram("latency_seconds", "help", metrics.DefaultBuckets, "method", "code")
	if c == nil || g == nil || h == nil {
		t.Fatal("Noop meter returned a nil metric")
	}
	if c.With("GET") == nil || g.With() == nil || h.With("GET", "200") == nil {
		t.Fatal("With returned nil")
	}
	// Mismatched label counts must not panic on the no-op implementation.
	c.With().Add(1)
	c.With("a", "b", "c").Add(1)
	g.Set(3)
	g.Add(-1)
	g.With("x").Set(1)
	h.Observe(0.2)
	h.With().Observe(1)
}

func TestNoopDoesNotAllocate(t *testing.T) {
	m := metrics.Noop()
	c := m.Counter("c", "h", "l")
	g := m.Gauge("g", "h", "l")
	h := m.Histogram("h", "h", nil, "l")
	allocs := testing.AllocsPerRun(100, func() {
		_ = metrics.Noop()
		c.With("v").Add(1)
		g.With("v").Set(1)
		g.With("v").Add(1)
		h.With("v").Observe(1)
	})
	if allocs != 0 {
		t.Fatalf("noop metrics allocated %v times per run, want 0", allocs)
	}
}

func TestNoopConcurrentUse(t *testing.T) {
	m := metrics.Noop()
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			m.Counter("c", "h", "l").With("v").Add(float64(i))
			m.Gauge("g", "h").Set(float64(i))
			m.Histogram("h", "h", metrics.DefaultBuckets).Observe(float64(i))
		})
	}
	wg.Wait()
}

func BenchmarkNoopCounter(b *testing.B) {
	c := metrics.Noop().Counter("c", "h", "l")
	for b.Loop() {
		c.With("v").Add(1)
	}
}
