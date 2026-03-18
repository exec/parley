package reporter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"parley/bench/internal/metrics"
)

// Terminal prints live progress and a final summary to stdout.
type Terminal struct {
	jsonMode bool
}

func New(jsonMode bool) *Terminal {
	return &Terminal{jsonMode: jsonMode}
}

// Progress prints a status line, overwriting the previous one.
// Use this for live updates during a run.
func (t *Terminal) Progress(format string, args ...any) {
	if t.jsonMode {
		return
	}
	fmt.Printf("\r\033[K"+format, args...)
}

// Println prints a line with a newline, preserving the terminal output.
func (t *Terminal) Println(format string, args ...any) {
	if t.jsonMode {
		return
	}
	fmt.Printf("\r\033[K"+format+"\n", args...)
}

// Summary prints the final report. In JSON mode it writes machine-readable JSON.
func (t *Terminal) Summary(scenario string, r *metrics.Report) {
	if t.jsonMode {
		printJSON(scenario, r)
		return
	}
	printHuman(scenario, r)
}

func printHuman(scenario string, r *metrics.Report) {
	fmt.Printf("\n\n=== %s — %.1fs ===\n\n", scenario, r.Duration.Seconds())

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "OPERATION\tCOUNT\tRPS\tp50\tp95\tp99\tERRORS")
	fmt.Fprintln(w, "---------\t-----\t---\t---\t---\t---\t------")

	for name, op := range r.Ops {
		errTotal := int64(0)
		errDetail := ""
		for code, n := range op.StatusCodes {
			if code >= 400 {
				errTotal += n
			}
		}
		// Build error breakdown string
		for code, n := range op.StatusCodes {
			if code >= 400 {
				errDetail += fmt.Sprintf(" %s:%d", http.StatusText(code), n)
			}
		}
		fmt.Fprintf(w, "%s\t%d\t%.1f/s\t%s\t%s\t%s\t%d%s\n",
			name, op.Count, op.RPS,
			fmtDur(op.P50), fmtDur(op.P95), fmtDur(op.P99),
			errTotal, errDetail,
		)
	}
	w.Flush()

	fmt.Printf("\nWebSocket disconnects: %d  |  send-buffer evictions: %d\n",
		r.WSDisconns, r.WSEvictions)

	if r.BroadcastSamples > 0 {
		fmt.Printf("Broadcast latency (%d samples) — p50:%s  p95:%s  p99:%s\n",
			r.BroadcastSamples,
			fmtDur(r.BroadcastP50), fmtDur(r.BroadcastP95), fmtDur(r.BroadcastP99),
		)
	}
	fmt.Println()
}

func printJSON(scenario string, r *metrics.Report) {
	out := map[string]any{
		"scenario":          scenario,
		"duration_s":        r.Duration.Seconds(),
		"operations":        r.Ops,
		"ws_disconnects":    r.WSDisconns,
		"ws_evictions":      r.WSEvictions,
		"broadcast_p50_ns":  r.BroadcastP50.Nanoseconds(),
		"broadcast_p95_ns":  r.BroadcastP95.Nanoseconds(),
		"broadcast_p99_ns":  r.BroadcastP99.Nanoseconds(),
		"broadcast_samples": r.BroadcastSamples,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

func fmtDur(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
}
