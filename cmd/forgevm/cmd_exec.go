package main

import (
	"fmt"
	"strings"

	"github.com/DohaerisAI/forgevm/internal/orchestrator"
	"github.com/spf13/cobra"
)

func newExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec <sandbox-id> -- <command>",
		Short: "Execute a command in a sandbox",
		Example: `  forgevm exec sb-a1b2c3d4 -- echo hello
  forgevm exec sb-a1b2c3d4 -- ls -la`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sandboxID := args[0]

			// Find everything after "--"
			var command string
			dashIdx := cmd.ArgsLenAtDash()
			if dashIdx >= 0 && dashIdx < len(args) {
				command = strings.Join(args[dashIdx:], " ")
			} else if len(args) > 1 {
				command = strings.Join(args[1:], " ")
			} else {
				return fmt.Errorf("no command specified; use: forgevm exec <id> -- <command>")
			}

			c := getClient()
			resp, err := c.do("POST", "/api/v1/sandboxes/"+sandboxID+"/exec", orchestrator.ExecRequest{
				Command: command,
			})
			if err != nil {
				return err
			}

			var result orchestrator.ExecResult
			if err := c.decodeJSON(resp, &result); err != nil {
				return err
			}

			if result.Stdout != "" {
				fmt.Print(result.Stdout)
			}
			if result.Stderr != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s", result.Stderr)
			}
			if result.ExitCode != 0 {
				return fmt.Errorf("exit code: %d", result.ExitCode)
			}
			return nil
		},
	}
	return cmd
}
