package metrics_test

import (
	"testing"
	"time"

	"parley/bench/internal/metrics"
)

func TestHistogramPercentiles(t *testing.T) {
	h := metrics.NewHistogram()

	// Record 100 samples: 1ms, 2ms, ..., 100ms
	for i := 1; i <= 100; i++ {
		h.Record(time.Duration(i) * time.Millisecond)
	}

	p50 := h.Percentile(50)
	if p50 < 49*time.Millisecond || p50 > 51*time.Millisecond {
		t.Errorf("p50 = %v, want ~50ms", p50)
	}

	p95 := h.Percentile(95)
	if p95 < 94*time.Millisecond || p95 > 96*time.Millisecond {
		t.Errorf("p95 = %v, want ~95ms", p95)
	}

	p99 := h.Percentile(99)
	if p99 < 98*time.Millisecond || p99 > 100*time.Millisecond {
		t.Errorf("p99 = %v, want ~99ms", p99)
	}
}

func TestHistogramEmpty(t *testing.T) {
	h := metrics.NewHistogram()
	if h.Percentile(50) != 0 {
		t.Error("empty histogram should return 0")
	}
	if h.Count() != 0 {
		t.Error("empty histogram count should be 0")
	}
}

func TestHistogramCount(t *testing.T) {
	h := metrics.NewHistogram()
	for i := 0; i < 42; i++ {
		h.Record(time.Millisecond)
	}
	if h.Count() != 42 {
		t.Errorf("count = %d, want 42", h.Count())
	}
}
