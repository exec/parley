package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"parley/bench/internal/client"
	"parley/bench/internal/metrics"
	"parley/bench/internal/reporter"
)

type BroadcastAmpOptions struct {
	Options
	Listeners int
	Rate      float64 // messages per second (writer)
	Duration  time.Duration
}

// benchMessage is embedded in message content to carry the send timestamp.
// The UI ignores unknown JSON fields, so this is safe to send to real servers.
type benchMessage struct {
	BenchTS int64  `json:"_bench_ts"`
	Text    string `json:"t"`
}

// RunBroadcastAmp runs the fan-out stress test: 1 writer, N listeners.
// Measures the time from HTTP POST → WS MESSAGE_CREATE receipt at each listener.
// IMPORTANT: run on the same host as the server to avoid clock skew.
func RunBroadcastAmp(ctx context.Context, opts BroadcastAmpOptions) error {
	col := metrics.NewCollector()
	rep := reporter.New(opts.JSONOutput)

	// Warn if not running against localhost — clock skew will corrupt latency numbers.
	if !strings.Contains(opts.Host, "localhost") && !strings.Contains(opts.Host, "127.0.0.1") {
		rep.Println("WARNING: host is not localhost — broadcast latency measurements may be skewed by clock differences.")
	}

	total := opts.Listeners + 1 // +1 for the writer
	rep.Println("Provisioning %d users (%d listeners + 1 writer)...", total, opts.Listeners)
	pr, err := client.Provision(ctx, opts.Host, opts.BenchSecret, "bench_broadamp_", total)
	if err != nil {
		return fmt.Errorf("provision: %w", err)
	}
	channelID := pr.ChannelID

	if opts.Cleanup {
		defer func() {
			n, _ := client.Cleanup(context.Background(), opts.Host, opts.BenchSecret, "bench_broadamp_")
			rep.Println("Cleaned up %d test accounts", n)
		}()
	}

	// Connect listeners and subscribe to the channel — all before the writer starts.
	rep.Println("Connecting %d listeners and subscribing to channel %d...", opts.Listeners, channelID)
	listeners := make([]*client.VirtualUser, opts.Listeners)
	var listenWG sync.WaitGroup
	var listenMu sync.Mutex
	connectErrors := 0

	for i := 0; i < opts.Listeners; i++ {
		listenWG.Add(1)
		go func(idx int) {
			defer listenWG.Done()
			u := client.NewVirtualUser(pr.Users[idx].ID, pr.Users[idx].Username, pr.Users[idx].Token, opts.Host, opts.BenchSecret)
			if err := u.ConnectWS(ctx, opts.Host); err != nil {
				col.IncWSDisconn()
				listenMu.Lock()
				connectErrors++
				listenMu.Unlock()
				return
			}
			if err := u.Subscribe(channelID); err != nil {
				u.Disconnect() // connected but failed to subscribe — close the WS
				col.IncWSDisconn()
				listenMu.Lock()
				connectErrors++
				listenMu.Unlock()
				return
			}
			listenMu.Lock()
			listeners[idx] = u
			listenMu.Unlock()

			// Monitor for eviction: if the server closes the conn due to full send buffer.
			go func() {
				<-u.WS.Done()
				col.IncWSDisconn()
			}()
		}(i)
	}
	listenWG.Wait()

	rep.Println("Listeners connected: %d/%d (connect errors: %d)",
		opts.Listeners-connectErrors, opts.Listeners, connectErrors)

	// Start per-listener broadcast latency measurement.
	for _, u := range listeners {
		if u == nil {
			continue
		}
		go func(u *client.VirtualUser) {
			for {
				select {
				case <-u.WS.Done():
					return
				case ev := <-u.WS.Events():
					if ev.Type != "MESSAGE_CREATE" {
						continue
					}
					receiveNS := time.Now().UnixNano()
					// Parse the message content to extract bench timestamp.
					var payload struct {
						Content string `json:"content"`
					}
					if err := json.Unmarshal(ev.Payload, &payload); err != nil {
						continue
					}
					var bm benchMessage
					if err := json.Unmarshal([]byte(payload.Content), &bm); err != nil || bm.BenchTS == 0 {
						continue
					}
					latency := time.Duration(receiveNS - bm.BenchTS)
					if latency > 0 && latency < 30*time.Second {
						col.RecordBroadcast(latency)
					}
				}
			}
		}(u)
	}

	// Writer: 1 user sends messages at the configured rate.
	writer := client.NewVirtualUser(
		pr.Users[total-1].ID,
		pr.Users[total-1].Username,
		pr.Users[total-1].Token,
		opts.Host, opts.BenchSecret,
	)

	if opts.Rate <= 0 {
		opts.Rate = 1
	}
	deadline := time.Now().Add(opts.Duration)
	interval := time.Duration(float64(time.Second) / opts.Rate)

	rep.Println("Writer started. Running for %s at %.0f msg/s with %d listeners...",
		opts.Duration, opts.Rate, opts.Listeners)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	progressTicker := time.NewTicker(2 * time.Second)
	defer progressTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			goto done
		case <-progressTicker.C:
			r := col.Report()
			sentCount := int64(0)
			if op, ok := r.Ops["send_message"]; ok {
				sentCount = op.Count
			}
			rep.Progress("listeners:%d  msg_sent:%d  broadcast_p50:%s  p99:%s  disconnects:%d",
				opts.Listeners-connectErrors,
				sentCount,
				formatDuration(r.BroadcastP50),
				formatDuration(r.BroadcastP99),
				r.WSDisconns,
			)
		case <-ticker.C:
			if time.Now().After(deadline) {
				goto done
			}
			// Embed send timestamp in message content.
			bm := benchMessage{BenchTS: time.Now().UnixNano(), Text: "amp"}
			content, _ := json.Marshal(bm)
			start := time.Now()
			status, err := writer.HTTP.SendMessage(ctx, channelID, string(content))
			if err != nil {
				col.Record("send_message", time.Since(start), 0)
				continue
			}
			if status != http.StatusCreated {
				col.Record("send_message", time.Since(start), status)
				continue
			}
			col.Record("send_message", time.Since(start), status)
		}
	}

done:
	// Disconnect all listeners.
	for _, u := range listeners {
		if u != nil {
			u.Disconnect()
		}
	}

	rep.Summary("broadcast-amp", col.Report())
	r := col.Report()
	rep.Println("\nBroadcast latency (%d samples): p50=%s  p95=%s  p99=%s",
		r.BroadcastSamples,
		formatDuration(r.BroadcastP50),
		formatDuration(r.BroadcastP95),
		formatDuration(r.BroadcastP99),
	)
	rep.Println("Fan-out mechanism: BroadcastToChannel holds hub mutex for all %d listeners.", opts.Listeners)
	rep.Println("Watch for: listener disconnect rate increases as hub mutex contention grows.")
	return nil
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "n/a"
	}
	if d >= time.Second {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	if d >= time.Millisecond {
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	}
	return fmt.Sprintf("%dµs", d.Microseconds())
}
