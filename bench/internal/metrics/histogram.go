package metrics

import (
	"sort"
	"sync"
	"time"
)

// Histogram records latency samples and computes percentiles.
// It copies and sorts samples on each Percentile call, which is fine
// for a bench tool where reporting is infrequent vs recording.
type Histogram struct {
	mu      sync.Mutex
	samples []time.Duration
}

func NewHistogram() *Histogram {
	return &Histogram{}
}

// Record adds a latency sample.
func (h *Histogram) Record(d time.Duration) {
	h.mu.Lock()
	h.samples = append(h.samples, d)
	h.mu.Unlock()
}

// Percentile returns the value at the given percentile (0–100).
// Returns 0 if no samples have been recorded.
func (h *Histogram) Percentile(p float64) time.Duration {
	h.mu.Lock()
	if len(h.samples) == 0 {
		h.mu.Unlock()
		return 0
	}
	sorted := make([]time.Duration, len(h.samples))
	copy(sorted, h.samples)
	h.mu.Unlock()

	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted)-1) * p / 100.0)
	return sorted[idx]
}

// Count returns the number of recorded samples.
func (h *Histogram) Count() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return int64(len(h.samples))
}
