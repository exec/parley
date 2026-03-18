package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// Collector accumulates per-operation stats and WS counters.
type Collector struct {
	start            time.Time
	mu               sync.Mutex
	ops              map[string]*opStats
	wsDisconns       atomic.Int64
	wsEvictions      atomic.Int64
	broadcastLatency *Histogram
}

type opStats struct {
	mu          sync.Mutex
	count       int64
	statusCodes map[int]int64
	latency     *Histogram
}

// OpReport is the computed summary for one operation type.
type OpReport struct {
	Count       int64
	RPS         float64
	P50         time.Duration
	P95         time.Duration
	P99         time.Duration
	StatusCodes map[int]int64 // counts by HTTP status code (all codes, including 2xx)
}

// Report is the full summary returned by Collector.Report().
type Report struct {
	Duration         time.Duration
	Ops              map[string]*OpReport
	WSDisconns       int64
	WSEvictions      int64
	BroadcastP50     time.Duration
	BroadcastP95     time.Duration
	BroadcastP99     time.Duration
	BroadcastSamples int64
}

func NewCollector() *Collector {
	return &Collector{
		start:            time.Now(),
		ops:              make(map[string]*opStats),
		broadcastLatency: NewHistogram(),
	}
}

// Record records a completed HTTP (or logical) operation with its latency and status code.
func (c *Collector) Record(op string, d time.Duration, statusCode int) {
	c.mu.Lock()
	s, ok := c.ops[op]
	if !ok {
		s = &opStats{
			statusCodes: make(map[int]int64),
			latency:     NewHistogram(),
		}
		c.ops[op] = s
	}
	c.mu.Unlock()

	s.mu.Lock()
	s.count++
	s.statusCodes[statusCode]++
	s.latency.Record(d)
	s.mu.Unlock()
}

// RecordBroadcast records a broadcast latency sample (HTTP POST → WS receive).
func (c *Collector) RecordBroadcast(d time.Duration) {
	c.broadcastLatency.Record(d)
}

// IncWSDisconn increments the WebSocket forcible-disconnect counter.
func (c *Collector) IncWSDisconn() { c.wsDisconns.Add(1) }

// IncWSEviction increments the WS client-send-buffer-full eviction counter.
func (c *Collector) IncWSEviction() { c.wsEvictions.Add(1) }

// Report computes and returns the final summary.
func (c *Collector) Report() *Report {
	elapsed := time.Since(c.start)
	elapsedSec := elapsed.Seconds()
	if elapsedSec < 0.001 {
		elapsedSec = 0.001
	}

	c.mu.Lock()
	opReports := make(map[string]*OpReport, len(c.ops))
	for name, s := range c.ops {
		s.mu.Lock()
		codes := make(map[int]int64, len(s.statusCodes))
		for k, v := range s.statusCodes {
			codes[k] = v
		}
		opReports[name] = &OpReport{
			Count:       s.count,
			RPS:         float64(s.count) / elapsedSec,
			P50:         s.latency.Percentile(50),
			P95:         s.latency.Percentile(95),
			P99:         s.latency.Percentile(99),
			StatusCodes: codes,
		}
		s.mu.Unlock()
	}
	c.mu.Unlock()

	return &Report{
		Duration:         elapsed,
		Ops:              opReports,
		WSDisconns:       c.wsDisconns.Load(),
		WSEvictions:      c.wsEvictions.Load(),
		BroadcastP50:     c.broadcastLatency.Percentile(50),
		BroadcastP95:     c.broadcastLatency.Percentile(95),
		BroadcastP99:     c.broadcastLatency.Percentile(99),
		BroadcastSamples: c.broadcastLatency.Count(),
	}
}
