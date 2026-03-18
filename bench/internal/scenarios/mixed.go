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

type MixedOptions struct {
	Options
	Users    int
	Duration time.Duration
}

// RunMixed runs a realistic mixed load: 20% writers, 60% readers, 20% typing spammers.
// All users are server members and subscribed to the channel before the sustain phase.
func RunMixed(ctx context.Context, opts MixedOptions) error {
	col := metrics.NewCollector()
	rep := reporter.New(opts.JSONOutput)

	rep.Println("Provisioning %d users...", opts.Users)
	pr, err := client.Provision(ctx, opts.Host, opts.BenchSecret, "bench_mixed_", opts.Users)
	if err != nil {
		return fmt.Errorf("provision: %w", err)
	}
	channelID := pr.ChannelID

	if opts.Cleanup {
		defer func() {
			n, _ := client.Cleanup(context.Background(), opts.Host, opts.BenchSecret, "bench_mixed_")
			rep.Println("Cleaned up %d test accounts", n)
		}()
	}

	// Connect all users to WS and subscribe to the channel before the sustain phase begins.
	rep.Println("Connecting all %d users to WebSocket and subscribing to channel...", opts.Users)
	users := pr.Users
	for _, u := range users {
		if err := u.ConnectWS(ctx, opts.Host); err != nil {
			rep.Println("WARNING: WS connect failed for user %s: %v", u.Username, err)
			col.IncWSDisconn()
			continue
		}
		if err := u.Subscribe(channelID); err != nil {
			rep.Println("WARNING: subscribe failed for user %s: %v", u.Username, err)
		}
		go func(u *client.VirtualUser) {
			<-u.WS.Done()
			col.IncWSDisconn()
		}(u)
	}
	defer func() {
		for _, u := range users {
			u.Disconnect()
		}
	}()

	start := time.Now()

	// Assign roles: 20% writers, 60% readers, 20% typing spammers.
	nWriters := opts.Users / 5
	nTypers := opts.Users / 5
	// nReaders is the rest

	writers := users[:nWriters]
	typers := users[nWriters : nWriters+nTypers]
	readers := users[nWriters+nTypers:]

	deadline := time.Now().Add(opts.Duration)
	rep.Println("Running mixed: %d writers, %d readers, %d typers for %s",
		len(writers), len(readers), len(typers), opts.Duration)

	var wg sync.WaitGroup

	// Writers: 1 msg/s
	for _, u := range writers {
		wg.Add(1)
		go func(u *client.VirtualUser) {
			defer wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if time.Now().After(deadline) || ctx.Err() != nil {
						return
					}
					bm := benchMessage{BenchTS: time.Now().UnixNano(), Text: "mixed"}
					content, _ := json.Marshal(bm)
					reqStart := time.Now()
					status, err := u.HTTP.SendMessage(ctx, channelID, string(content))
					if err != nil {
						col.Record("send_message", time.Since(reqStart), 0)
						continue
					}
					col.Record("send_message", time.Since(reqStart), status)
				case <-ctx.Done():
					return
				}
			}
		}(u)
	}

	// Readers: tight-loop GET /messages
	for _, u := range readers {
		wg.Add(1)
		go func(u *client.VirtualUser) {
			defer wg.Done()
			c := client.NewHTTPClient(opts.Host, u.Token, opts.BenchSecret)
			for time.Now().Before(deadline) && ctx.Err() == nil {
				reqStart := time.Now()
				status, err := c.GetMessages(ctx, channelID)
				if err != nil {
					col.Record("get_messages", time.Since(reqStart), 0)
					continue
				}
				col.Record("get_messages", time.Since(reqStart), status)
			}
		}(u)
	}

	// Typers: TYPING WS event at 2/s
	for _, u := range typers {
		wg.Add(1)
		go func(u *client.VirtualUser) {
			defer wg.Done()
			if u.WS == nil {
				return
			}
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if time.Now().After(deadline) || ctx.Err() != nil {
						return
					}
					_ = u.WS.SendTyping(channelID)
				case <-u.WS.Done():
					return
				case <-ctx.Done():
					return
				}
			}
		}(u)
	}

	// Broadcast latency listener (uses first writer's WS connection)
	if len(writers) > 0 && writers[0].WS != nil {
		go func() {
			for {
				select {
				case <-writers[0].WS.Done():
					return
				case ev := <-writers[0].WS.Events():
					if ev.Type != "MESSAGE_CREATE" {
						continue
					}
					var payload struct{ Content string `json:"content"` }
					json.Unmarshal(ev.Payload, &payload)
					var bm benchMessage
					if json.Unmarshal([]byte(payload.Content), &bm) == nil && bm.BenchTS > 0 {
						latency := time.Duration(time.Now().UnixNano() - bm.BenchTS)
						if latency > 0 && latency < 30*time.Second {
							col.RecordBroadcast(latency)
						}
					}
				}
			}
		}()
	}

	// Progress reporter
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				r := col.Report()
				sendRPS, readRPS := 0.0, 0.0
				if op, ok := r.Ops["send_message"]; ok {
					sendRPS = op.RPS
				}
				if op, ok := r.Ops["get_messages"]; ok {
					readRPS = op.RPS
				}
				rep.Progress("send:%.1f/s  read:%.1f/s  ws_disc:%d  b_p99:%s  elapsed:%.0fs",
					sendRPS, readRPS, r.WSDisconns, formatDuration(r.BroadcastP99),
					time.Since(start).Seconds(),
				)
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	rep.Summary("mixed", col.Report())
	return nil
}
