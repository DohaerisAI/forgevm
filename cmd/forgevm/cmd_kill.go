package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newKillCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "kill <sandbox-id>",
		Short:   "Destroy a sandbox",
		Example: "  forgevm kill sb-a1b2c3d4",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			resp, err := c.do("DELETE", "/api/v1/sandboxes/"+args[0], nil)
			if err != nil {
				return err
			}

			var result map[string]string
			if err := c.decodeJSON(resp, &result); err != nil {
				return err
			}

			fmt.Printf("Sandbox %s destroyed\n", args[0])
			return nil
		},
	}
}
