package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const productionHost = "parley.x86-64.com"

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
		Long:  "Stress-tests Parley's HTTP and WebSocket APIs to find bottlenecks.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			host, _ := cmd.Flags().GetString("host")
			if strings.Contains(host, productionHost) {
				return fmt.Errorf("refusing to run against production host %q — use a dev instance", productionHost)
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

// Placeholder subcommand stubs — implemented in later tasks.
// Each file in this package defines one XxxCmd() function.
func authFloodCmd() *cobra.Command {
	return &cobra.Command{Use: "auth-flood", Short: "Hammer auth endpoints", RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("auth-flood not yet implemented")
	}}
}
func wsScaleCmd() *cobra.Command {
	return &cobra.Command{Use: "ws-scale", Short: "Ramp WebSocket connections", RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("ws-scale not yet implemented")
	}}
}
func messageStormCmd() *cobra.Command {
	return &cobra.Command{Use: "message-storm", Short: "N writers hammer a channel", RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("message-storm not yet implemented")
	}}
}
func broadcastAmpCmd() *cobra.Command {
	return &cobra.Command{Use: "broadcast-amp", Short: "1 writer + N listeners — measure broadcast latency", RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("broadcast-amp not yet implemented")
	}}
}
func readHeavyCmd() *cobra.Command {
	return &cobra.Command{Use: "read-heavy", Short: "Concurrent message history reads", RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("read-heavy not yet implemented")
	}}
}
func mixedCmd() *cobra.Command {
	return &cobra.Command{Use: "mixed", Short: "Realistic combined load", RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("mixed not yet implemented")
	}}
}
