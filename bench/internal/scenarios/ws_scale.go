package scenarios

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"parley/bench/internal/client"
	"parley/bench/internal/metrics"
	"parley/bench/internal/reporter"
)

type WSScaleOptions struct {
	Options
	Max        int
	RampRate   float64 // connections per second
	SustainFor time.Duration
}

// RunWSScale ramps up concurrent WebSocket connections to find the hub's connection cliff.
func RunWSScale(ctx context.Context, opts WSScaleOptions) error {
	col := metrics.NewCollector()
	rep := reporter.New(opts.JSONOutput)

	rep.Println("Provisioning %d test users...", opts.Max)
	pr, err := client.Provision(ctx, opts.Host, opts.BenchSecret, "bench_wsscale_", opts.Max)
	if err != nil {
		return fmt.Errorf("provision: %w", err)
	}

	if opts.Cleanup {
		defer func() {
			n, _ := client.Cleanup(context.Background(), opts.Host, opts.BenchSecret, "bench_wsscale_")
			rep.Println("Cleaned up %d test accounts", n)
		}()
	}

	var (
		connected    atomic.Int64
		disconnected atomic.Int64
		users        []*client.VirtualUser
		mu           sync.Mutex
	)

	rep.Println("Ramping up to %d WS connections at %.0f/s...", opts.Max, opts.RampRate)

	err = Ramp(ctx, opts.Max, opts.RampRate, func(i int) error {
		u := pr.Users[i]
		start := time.Now()
		if err := u.ConnectWS(ctx, opts.Host); err != nil {
			col.Record("ws_connect", time.Since(start), 0)
			col.IncWSDisconn()
			return nil // non-fatal: keep ramping
		}
		col.Record("ws_connect", time.Since(start), 101) // 101 Switching Protocols

		connected.Add(1)
		mu.Lock()
		users = append(users, u)
		mu.Unlock()

		// Monitor for disconnection in background.
		go func() {
			<-u.WS.Done()
			disconnected.Add(1)
			col.IncWSDisconn()
		}()

		rep.Progress("connected:%d/%d  disconnected:%d",
			connected.Load(), opts.Max, disconnected.Load())
		return nil
	})
	if err != nil {
		return err
	}

	rep.Println("\nRamp complete: %d connected, %d failed. Sustaining for %s...",
		connected.Load(), int64(opts.Max)-connected.Load(), opts.SustainFor)

	time.Sleep(opts.SustainFor)

	rep.Println("Draining connections...")
	mu.Lock()
	for _, u := range users {
		u.Disconnect()
	}
	mu.Unlock()

	rep.Summary("ws-scale", col.Report())
	rep.Println("Connection cliff: watch for disconnection rate > 0 during sustain phase.")
	return nil
}
