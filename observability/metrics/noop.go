package metrics

import "context"

// Noop returns a Meter whose metrics discard every observation. It does not
// allocate and is safe for concurrent use.
func Noop() Meter { return noopMeter{} }

// noopMeter is a Meter that creates no-op metrics.
type noopMeter struct{}

func (noopMeter) Counter(string, string, ...string) Counter { return noopCounter{} }

func (noopMeter) Gauge(string, string, ...string) Gauge { return noopGauge{} }

func (noopMeter) Histogram(string, string, []float64, ...string) Histogram {
	return noopHistogram{}
}

// noopCounter is a Counter that discards every update.
type noopCounter struct{}

func (c noopCounter) With(...string) Counter     { return c }
func (noopCounter) Add(context.Context, float64) {}

// noopGauge is a Gauge that discards every update.
type noopGauge struct{}

func (g noopGauge) With(...string) Gauge       { return g }
func (noopGauge) Set(context.Context, float64) {}
func (noopGauge) Add(context.Context, float64) {}

// noopHistogram is a Histogram that discards every observation.
type noopHistogram struct{}

func (h noopHistogram) With(...string) Histogram       { return h }
func (noopHistogram) Observe(context.Context, float64) {}
