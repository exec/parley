package metrics_test

import (
	"net/http"
	"testing"
	"time"

	"parley/bench/internal/metrics"
)

func TestCollectorRecord(t *testing.T) {
	c := metrics.NewCollector()

	c.Record("send_message", 10*time.Millisecond, http.StatusCreated)
	c.Record("send_message", 20*time.Millisecond, http.StatusCreated)
	c.Record("send_message", 30*time.Millisecond, http.StatusTooManyRequests)

	report := c.Report()
	op, ok := report.Ops["send_message"]
	if !ok {
		t.Fatal("send_message not in report")
	}
	if op.Count != 3 {
		t.Errorf("count = %d, want 3", op.Count)
	}
	if op.StatusCodes[http.StatusTooManyRequests] != 1 {
		t.Errorf("429 count = %d, want 1", op.StatusCodes[http.StatusTooManyRequests])
	}
	if op.P50 < 10*time.Millisecond || op.P50 > 30*time.Millisecond {
		t.Errorf("p50 = %v, out of expected range", op.P50)
	}
}

func TestCollectorRPS(t *testing.T) {
	c := metrics.NewCollector()
	for i := 0; i < 100; i++ {
		c.Record("get_messages", time.Millisecond, http.StatusOK)
	}
	// Elapsed is nearly 0 in the test, so RPS will be huge — just check it's positive.
	report := c.Report()
	if report.Ops["get_messages"].RPS <= 0 {
		t.Error("RPS should be positive")
	}
}

func TestCollectorWSCounters(t *testing.T) {
	c := metrics.NewCollector()
	c.IncWSDisconn()
	c.IncWSDisconn()
	c.IncWSEviction()

	report := c.Report()
	if report.WSDisconns != 2 {
		t.Errorf("WSDisconns = %d, want 2", report.WSDisconns)
	}
	if report.WSEvictions != 1 {
		t.Errorf("WSEvictions = %d, want 1", report.WSEvictions)
	}
}
