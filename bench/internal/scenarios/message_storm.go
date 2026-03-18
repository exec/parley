package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"parley/bench/internal/client"
	"parley/bench/internal/metrics"
	"parley/bench/internal/reporter"
)

type MessageStormOptions struct {
	Options
	Writers  int
	Rate     float64 // messages per second per writer
	Duration time.Duration
}

// RunMessageStorm has N writers hammer one channel. A pre-connected WS listener
// measures broadcast delivery lag. This stresses hub.BroadcastToChannel's
// mutex hold time under sustained write load.
func RunMessageStorm(ctx context.Context, opts MessageStormOptions) error {
	col := metrics.NewCollector()
	rep := reporter.New(opts.JSONOutput)

	total := opts.Writers + 1 // +1 for the WS listener
	rep.Println("Provisioning %d users (writers + 1 listener)...", total)
	pr, err := client.Provision(ctx, opts.Host, opts.BenchSecret, "bench_msgstorm_", total)
	if err != nil {
		return fmt.Errorf("provision: %w", err)
	}
	channelID := pr.ChannelID

	if opts.Cleanup {
		defer func() {
			n, _ := client.Cleanup(context.Background(), opts.Host, opts.BenchSecret, "bench_msgstorm_")
			rep.Println("Cleaned up %d test accounts", n)
		}()
	}

	// The last user is the WS listener. Connect it and subscribe before the storm.
	// Writers send via HTTP only — no WS connection needed for them.
	listener := pr.Users[total-1]
	if err := listener.ConnectWS(ctx, opts.Host); err != nil {
		return fmt.Errorf("connect listener WS: %w", err)
	}
	if err := listener.Subscribe(channelID); err != nil {
		return fmt.Errorf("subscribe listener: %w", err)
	}
	defer listener.Disconnect()

	// Track listener disconnection.
	go func() {
		<-listener.WS.Done()
		col.IncWSDisconn()
	}()

	// Record broadcast lag from the listener's event stream.
	// Writers embed {"_bench_ts": <nanoseconds>} in message content;
	// we parse it here to compute receive_ns - send_ns.
	go func() {
		for {
			select {
			case <-listener.WS.Done():
				return
			case ev := <-listener.WS.Events():
				if ev.Type != "MESSAGE_CREATE" {
					continue
				}
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
				latency := time.Duration(time.Now().UnixNano() - bm.BenchTS)
				if latency > 0 && latency < 30*time.Second {
					col.RecordBroadcast(latency)
				}
			}
		}
	}()

	deadline := time.Now().Add(opts.Duration)
	interval := time.Duration(float64(time.Second) / opts.Rate)

	rep.Println("Running message-storm: %d writers at %.0f msg/s for %s", opts.Writers, opts.Rate, opts.Duration)

	var wg sync.WaitGroup
	writers := pr.Users[:opts.Writers]
	for _, u := range writers {
		wg.Add(1)
		go func(u *client.VirtualUser) {
			defer wg.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if time.Now().After(deadline) || ctx.Err() != nil {
						return
					}
					bm := benchMessage{BenchTS: time.Now().UnixNano(), Text: "storm"}
					content, _ := json.Marshal(bm)
					start := time.Now()
					status, err := u.HTTP.SendMessage(ctx, channelID, string(content))
					if err != nil {
						col.Record("send_message", time.Since(start), 0)
						continue
					}
					col.Record("send_message", time.Since(start), status)
				case <-ctx.Done():
					return
				}
			}
		}(u)
	}

	// Progress reporter
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r := col.Report()
				var rps float64
				if op, ok := r.Ops["send_message"]; ok {
					rps = op.RPS
				}
				rep.Progress("writers:%d  msg_rps:%.1f  ws_disconnects:%d  evictions:%d",
					opts.Writers, rps, r.WSDisconns, r.WSEvictions)
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	rep.Summary("message-storm", col.Report())
	return nil
}
