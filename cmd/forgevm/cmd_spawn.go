package main

import (
	"fmt"

	"github.com/DohaerisAI/forgevm/internal/orchestrator"
	"github.com/spf13/cobra"
)

func newSpawnCmd() *cobra.Command {
	var (
		image    string
		provider string
		ttl      string
		memoryMB int
		vcpus    int
	)

	cmd := &cobra.Command{
		Use:   "spawn",
		Short: "Spawn a new sandbox",
		Example: `  forgevm spawn --image alpine:latest
  forgevm spawn --image ubuntu:22.04 --ttl 1h --memory 1024`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			resp, err := c.do("POST", "/api/v1/sandboxes", orchestrator.SpawnRequest{
				Image:    image,
				Provider: provider,
				TTL:      ttl,
				MemoryMB: memoryMB,
				VCPUs:    vcpus,
			})
			if err != nil {
				return err
			}

			var sb orchestrator.Sandbox
			if err := c.decodeJSON(resp, &sb); err != nil {
				return err
			}

			fmt.Printf("Sandbox %s spawned (provider: %s, image: %s)\n", sb.ID, sb.Provider, sb.Image)
			return nil
		},
	}

	cmd.Flags().StringVar(&image, "image", "alpine:latest", "Image to use")
	cmd.Flags().StringVar(&provider, "provider", "", "Provider (default: server default)")
	cmd.Flags().StringVar(&ttl, "ttl", "", "TTL duration (e.g. 30m, 1h)")
	cmd.Flags().IntVar(&memoryMB, "memory", 0, "Memory in MB")
	cmd.Flags().IntVar(&vcpus, "vcpus", 0, "Number of vCPUs")

	return cmd
}
