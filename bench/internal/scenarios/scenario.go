package scenarios

import (
	"context"
	"time"

	"parley/bench/internal/client"
	"parley/bench/internal/metrics"
	"parley/bench/internal/reporter"
)

// Options holds common configuration for all scenarios.
type Options struct {
	Host        string
	BenchSecret string
	Cleanup     bool
	JSONOutput  bool
}

// RunFunc is the signature every scenario implements.
type RunFunc func(ctx context.Context, opts Options, col *metrics.Collector, rep *reporter.Terminal) error

// Ramp calls spawn(i) at the given rate until count invocations, then returns.
// If ctx is cancelled, returns ctx.Err() immediately.
func Ramp(ctx context.Context, count int, ratePerSec float64, spawn func(i int) error) error {
	if ratePerSec <= 0 {
		ratePerSec = 10
	}
	interval := time.Duration(float64(time.Second) / ratePerSec)
	for i := 0; i < count; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := spawn(i); err != nil {
			return err
		}
		if i < count-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
		}
	}
	return nil
}

// ProvisionAndConnect provisions users, connects each to the WebSocket,
// and subscribes them to channelID. Returns the users ready for load.
// This is used by scenarios that need WS + channel subscription set up
// before the sustain phase begins.
func ProvisionAndConnect(ctx context.Context, opts Options, count int, channelID int64, prefix string) ([]*client.VirtualUser, *client.ProvisionResult, error) {
	pr, err := client.Provision(ctx, opts.Host, opts.BenchSecret, prefix, count)
	if err != nil {
		return nil, nil, err
	}

	for _, u := range pr.Users {
		if err := u.ConnectWS(ctx, opts.Host); err != nil {
			return nil, pr, err
		}
		if channelID > 0 {
			if err := u.Subscribe(channelID); err != nil {
				return nil, pr, err
			}
		}
	}
	return pr.Users, pr, nil
}
