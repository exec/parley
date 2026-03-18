package scenarios

import (
	"context"
	"fmt"
	"sync"
	"time"

	"parley/bench/internal/client"
	"parley/bench/internal/metrics"
	"parley/bench/internal/reporter"
)

type ReadHeavyOptions struct {
	Options
	Readers  int
	Duration time.Duration
}

// RunReadHeavy hits GET /api/channels/{id}/messages in a tight loop.
// Targets the 120/min rate limiter and DB read path under load.
func RunReadHeavy(ctx context.Context, opts ReadHeavyOptions) error {
	col := metrics.NewCollector()
	rep := reporter.New(opts.JSONOutput)

	rep.Println("Provisioning %d readers...", opts.Readers)
	pr, err := client.Provision(ctx, opts.Host, opts.BenchSecret, "bench_readheavy_", opts.Readers)
	if err != nil {
		return fmt.Errorf("provision: %w", err)
	}
	channelID := pr.ChannelID

	if opts.Cleanup {
		defer func() {
			n, _ := client.Cleanup(context.Background(), opts.Host, opts.BenchSecret, "bench_readheavy_")
			rep.Println("Cleaned up %d test accounts", n)
		}()
	}

	deadline := time.Now().Add(opts.Duration)
	rep.Println("Running read-heavy: %d readers against channel %d for %s", opts.Readers, channelID, opts.Duration)

	var wg sync.WaitGroup
	for _, u := range pr.Users {
		wg.Add(1)
		go func(u *client.VirtualUser) {
			defer wg.Done()
			c := client.NewHTTPClient(opts.Host, u.Token, opts.BenchSecret)
			for time.Now().Before(deadline) && ctx.Err() == nil {
				start := time.Now()
				status, err := c.GetMessages(ctx, channelID)
				if err != nil {
					col.Record("get_messages", time.Since(start), 0)
					continue
				}
				col.Record("get_messages", time.Since(start), status)
			}
		}(u)
	}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r := col.Report()
				if op, ok := r.Ops["get_messages"]; ok {
					rep.Progress("readers:%d  rps:%.1f  429s:%d  p99:%s",
						opts.Readers, op.RPS, op.StatusCodes[429], formatDuration(op.P99))
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	rep.Summary("read-heavy", col.Report())
	return nil
}
