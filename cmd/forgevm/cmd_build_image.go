package main

import (
	"fmt"
	"os"
	"time"

	"github.com/mainadwitiya/forgevm/internal/config"
	"github.com/mainadwitiya/forgevm/internal/providers"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

func newBuildImageCmd() *cobra.Command {
	var diskMB int

	cmd := &cobra.Command{
		Use:   "build-image <docker-image>",
		Short: "Build a Firecracker rootfs from a Docker image",
		Long: `Build a Firecracker-compatible ext4 rootfs from a Docker image.

The built rootfs is cached so subsequent sandbox spawns with the same image
are instant. The forgevm-agent binary is automatically injected into the image.

Requires: Docker (uses a privileged container for ext4 creation — no sudo needed).

Examples:
  forgevm build-image python:3.12-slim
  forgevm build-image node:20-alpine
  forgevm build-image ubuntu:22.04 --disk-size 2048`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBuildImage(args[0], diskMB)
		},
	}

	cmd.Flags().IntVar(&diskMB, "disk-size", 0, "Disk size in MB (default: 1024)")

	return cmd
}

func runBuildImage(image string, diskMB int) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if !cfg.Providers.Firecracker.Enabled {
		return fmt.Errorf("firecracker provider is not enabled in config")
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()

	fcCfg := cfg.Providers.Firecracker
	cache := providers.NewImageCache(
		fcCfg.DataDir+"/images",
		cfg.ResolveAgentPath(),
		diskMB,
		logger,
	)

	fmt.Printf("Building rootfs for %s...\n", image)
	start := time.Now()

	path, err := cache.Get(image)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("Done in %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Cached at: %s\n", path)
	return nil
}
