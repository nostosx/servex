package metrics

// Meter creates metrics. Calling a constructor again with the same name and
// kind returns an equivalent metric. Label names are declared at creation.
type Meter interface {
	// Counter returns a monotonically increasing counter.
	Counter(name, help string, labelNames ...string) Counter
	// Gauge returns a gauge whose value can go up and down.
	Gauge(name, help string, labelNames ...string) Gauge
	// Histogram returns a histogram with the given bucket upper bounds.
	Histogram(name, help string, buckets []float64, labelNames ...string) Histogram
}

// Counter is a monotonically increasing value.
type Counter interface {
	// With returns the counter bound to labelValues, given in the order of
	// the label names declared at creation.
	With(labelValues ...string) Counter
	// Add increases the counter by delta, which must not be negative.
	Add(delta float64)
}

// Gauge is a value that can go up and down.
type Gauge interface {
	// With returns the gauge bound to labelValues, given in the order of the
	// label names declared at creation.
	With(labelValues ...string) Gauge
	// Set sets the gauge to v.
	Set(v float64)
	// Add adds delta, which may be negative, to the gauge.
	Add(delta float64)
}

// Histogram samples observations into buckets.
type Histogram interface {
	// With returns the histogram bound to labelValues, given in the order of
	// the label names declared at creation.
	With(labelValues ...string) Histogram
	// Observe records v.
	Observe(v float64)
}

// DefaultBuckets are histogram bucket upper bounds in seconds, suited to
// request latencies. Treat it as read-only; copy it before modifying.
var DefaultBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
