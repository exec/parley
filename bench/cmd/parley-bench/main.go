package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"parley/bench/internal/scenarios"
)

// productionHost is the domain to block bench runs against.
// Override via PARLEY_PROD_DOMAIN env var when migrating to a new domain.
var productionHost = func() string {
	if v := os.Getenv("PARLEY_PROD_DOMAIN"); v != "" {
		return v
	}
	return os.Getenv("DOMAIN")
}()

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "parley-bench",
		Short: "Parley load testing CLI",
		Long:  "Stress-tests Parley HTTP and WebSocket APIs to find bottlenecks.\n\nTarget a dev/Proxmox instance — never production.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			host, _ := cmd.Flags().GetString("host")
			// Only block when a production host is actually configured —
			// strings.Contains(x, "") is always true, so an empty
			// productionHost would reject every target.
			if productionHost != "" && strings.Contains(host, productionHost) {
				return fmt.Errorf("refusing to run against production host %q", productionHost)
			}
			return nil
		},
	}

	root.PersistentFlags().String("host", "http://localhost:8080", "Target host URL")
	root.PersistentFlags().String("bench-secret", "", "X-Bench-Secret for provisioner endpoint")
	root.PersistentFlags().Bool("cleanup", true, "Delete test data after run")
	root.PersistentFlags().Bool("json", false, "Output results as JSON")

	root.AddCommand(authFloodCmd())
	root.AddCommand(wsScaleCmd())
	root.AddCommand(messageStormCmd())
	root.AddCommand(broadcastAmpCmd())
	root.AddCommand(readHeavyCmd())
	root.AddCommand(mixedCmd())

	return root
}

func commonOpts(cmd *cobra.Command) scenarios.Options {
	host, _ := cmd.Flags().GetString("host")
	secret, _ := cmd.Flags().GetString("bench-secret")
	cleanup, _ := cmd.Flags().GetBool("cleanup")
	jsonOut, _ := cmd.Flags().GetBool("json")
	return scenarios.Options{
		Host: host, BenchSecret: secret, Cleanup: cleanup, JSONOutput: jsonOut,
	}
}

func runWithSignal(f func(context.Context) error) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nInterrupted — draining...")
		cancel()
	}()
	return f(ctx)
}

func authFloodCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth-flood",
		Short: "Hammer auth endpoints — measure bcrypt latency and rate limiter behaviour",
		RunE: func(cmd *cobra.Command, args []string) error {
			workers, _ := cmd.Flags().GetInt("workers")
			dur, _ := cmd.Flags().GetDuration("duration")
			return runWithSignal(func(ctx context.Context) error {
				return scenarios.RunAuthFlood(ctx, scenarios.AuthFloodOptions{
					Options:  commonOpts(cmd),
					Workers:  workers,
					Duration: dur,
				})
			})
		},
	}
	cmd.Flags().Int("workers", 20, "Concurrent goroutines")
	cmd.Flags().Duration("duration", 5*time.Minute, "Sustain duration")
	return cmd
}

func wsScaleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ws-scale",
		Short: "Ramp WebSocket connections — find the hub's connection cliff",
		RunE: func(cmd *cobra.Command, args []string) error {
			max, _ := cmd.Flags().GetInt("max")
			rate, _ := cmd.Flags().GetFloat64("ramp-rate")
			sustain, _ := cmd.Flags().GetDuration("sustain")
			return runWithSignal(func(ctx context.Context) error {
				return scenarios.RunWSScale(ctx, scenarios.WSScaleOptions{
					Options:    commonOpts(cmd),
					Max:        max,
					RampRate:   rate,
					SustainFor: sustain,
				})
			})
		},
	}
	cmd.Flags().Int("max", 1000, "Maximum concurrent WS connections")
	cmd.Flags().Float64("ramp-rate", 10, "New connections per second during ramp")
	cmd.Flags().Duration("sustain", 3*time.Minute, "How long to hold at max connections")
	return cmd
}

func messageStormCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "message-storm",
		Short: "N writers hammer one channel — measure hub mutex contention",
		RunE: func(cmd *cobra.Command, args []string) error {
			writers, _ := cmd.Flags().GetInt("writers")
			rate, _ := cmd.Flags().GetFloat64("rate")
			dur, _ := cmd.Flags().GetDuration("duration")
			return runWithSignal(func(ctx context.Context) error {
				return scenarios.RunMessageStorm(ctx, scenarios.MessageStormOptions{
					Options:  commonOpts(cmd),
					Writers:  writers,
					Rate:     rate,
					Duration: dur,
				})
			})
		},
	}
	cmd.Flags().Int("writers", 50, "Number of concurrent message writers")
	cmd.Flags().Float64("rate", 1, "Messages per second per writer")
	cmd.Flags().Duration("duration", 10*time.Minute, "Sustain duration")
	return cmd
}

func broadcastAmpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "broadcast-amp",
		Short: "1 writer + N listeners — measure broadcast fan-out latency",
		Long: `Measures time from HTTP POST to WebSocket MESSAGE_CREATE receipt.
Run on the same host as the server to avoid clock skew corrupting measurements.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			listeners, _ := cmd.Flags().GetInt("listeners")
			rate, _ := cmd.Flags().GetFloat64("rate")
			dur, _ := cmd.Flags().GetDuration("duration")
			return runWithSignal(func(ctx context.Context) error {
				return scenarios.RunBroadcastAmp(ctx, scenarios.BroadcastAmpOptions{
					Options:   commonOpts(cmd),
					Listeners: listeners,
					Rate:      rate,
					Duration:  dur,
				})
			})
		},
	}
	cmd.Flags().Int("listeners", 500, "Number of WS listeners")
	cmd.Flags().Float64("rate", 1, "Messages per second (writer)")
	cmd.Flags().Duration("duration", 10*time.Minute, "Sustain duration")
	return cmd
}

func readHeavyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "read-heavy",
		Short: "Concurrent message history reads — hit the 120/min rate limiter",
		RunE: func(cmd *cobra.Command, args []string) error {
			readers, _ := cmd.Flags().GetInt("readers")
			dur, _ := cmd.Flags().GetDuration("duration")
			return runWithSignal(func(ctx context.Context) error {
				return scenarios.RunReadHeavy(ctx, scenarios.ReadHeavyOptions{
					Options:  commonOpts(cmd),
					Readers:  readers,
					Duration: dur,
				})
			})
		},
	}
	cmd.Flags().Int("readers", 20, "Concurrent readers")
	cmd.Flags().Duration("duration", 5*time.Minute, "Sustain duration")
	return cmd
}

func mixedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mixed",
		Short: "Realistic combined load — writers + readers + typing (15m default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			users, _ := cmd.Flags().GetInt("users")
			dur, _ := cmd.Flags().GetDuration("duration")
			return runWithSignal(func(ctx context.Context) error {
				return scenarios.RunMixed(ctx, scenarios.MixedOptions{
					Options:  commonOpts(cmd),
					Users:    users,
					Duration: dur,
				})
			})
		},
	}
	cmd.Flags().Int("users", 200, "Total virtual users (20% writers, 60% readers, 20% typers)")
	cmd.Flags().Duration("duration", 15*time.Minute, "Sustain duration")
	return cmd
}
