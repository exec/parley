package scenarios

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"parley/bench/internal/client"
	"parley/bench/internal/metrics"
	"parley/bench/internal/reporter"
)

// AuthFloodOptions are the auth-flood-specific flags.
type AuthFloodOptions struct {
	Options
	Workers  int
	Duration time.Duration
}

// RunAuthFlood hammers login endpoints from N workers.
// Note: the server's rate limiter caps to 10 req/min per IP from a single machine.
// This scenario measures the cap response (429 rate) and bcrypt latency
// for the requests that succeed.
func RunAuthFlood(ctx context.Context, opts AuthFloodOptions) error {
	col := metrics.NewCollector()
	rep := reporter.New(opts.JSONOutput)

	// Pre-create one real user to log in as (via provision endpoint or REST).
	rep.Println("Provisioning 1 test user...")
	pr, err := client.Provision(ctx, opts.Host, opts.BenchSecret, "bench_authflood_", 1)
	if err != nil {
		return fmt.Errorf("provision: %w", err)
	}
	testUsername := pr.Users[0].Username
	testEmail := testUsername + "@bench.invalid"

	if opts.Cleanup {
		defer func() {
			cleanCtx := context.Background()
			n, _ := client.Cleanup(cleanCtx, opts.Host, opts.BenchSecret, "bench_authflood_")
			rep.Println("Cleaned up %d test accounts", n)
		}()
	}

	deadline := time.Now().Add(opts.Duration)
	start := time.Now()
	rep.Println("Running auth-flood: %d workers for %s", opts.Workers, opts.Duration)
	rep.Println("Note: rate limiter caps to 10 req/min from one IP — 429s are expected.")

	var wg sync.WaitGroup
	for i := 0; i < opts.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			// Alternate valid/invalid credentials.
			c := client.NewHTTPClient(opts.Host, "", opts.BenchSecret)
			iteration := 0
			for time.Now().Before(deadline) {
				if ctx.Err() != nil {
					return
				}
				reqStart := time.Now()
				var status int
				var err error
				if iteration%3 == 0 {
					// Invalid credentials
					_, status, err = c.Login(ctx, "nobody@invalid.test", "wrongpassword")
				} else {
					// Valid credentials
					_, status, err = c.Login(ctx, testEmail, "benchtest")
				}
				elapsed := time.Since(reqStart)
				iteration++
				if err != nil {
					col.Record("login", elapsed, 0)
					continue
				}
				col.Record("login", elapsed, status)

				elapsedSec := time.Since(start).Seconds()
				rep.Progress("workers:%d  elapsed:%.0fs  rps:%.1f",
					opts.Workers,
					elapsedSec,
					float64(col.Report().Ops["login"].Count)/elapsedSec,
				)
			}
		}(i)
	}

	wg.Wait()
	rep.Summary("auth-flood", col.Report())
	rep.Println("Tip: 429 count shows the rate limiter working. bcrypt p99 shows server CPU cost per auth.")
	if col.Report().Ops["login"] != nil {
		codes := col.Report().Ops["login"].StatusCodes
		if codes[http.StatusTooManyRequests] > 0 {
			pct := float64(codes[http.StatusTooManyRequests]) / float64(col.Report().Ops["login"].Count) * 100
			rep.Println("  429 rate: %.1f%% (rate limiter hit)", pct)
		}
	}
	return nil
}
