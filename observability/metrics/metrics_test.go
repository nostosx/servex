package metrics_test

import (
	"slices"
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
