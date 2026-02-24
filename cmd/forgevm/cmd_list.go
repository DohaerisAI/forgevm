package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/mainadwitiya/forgevm/internal/orchestrator"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List active sandboxes",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			resp, err := c.do("GET", "/api/v1/sandboxes", nil)
			if err != nil {
				return err
			}

			var sandboxes []orchestrator.Sandbox
			if err := c.decodeJSON(resp, &sandboxes); err != nil {
				return err
			}

			if len(sandboxes) == 0 {
				fmt.Println("No active sandboxes")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSTATE\tPROVIDER\tIMAGE\tCREATED\tEXPIRES")
			for _, sb := range sandboxes {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					sb.ID,
					sb.State,
					sb.Provider,
					sb.Image,
					sb.CreatedAt.Format("15:04:05"),
					sb.ExpiresAt.Format("15:04:05"),
				)
			}
			return w.Flush()
		},
	}
}
