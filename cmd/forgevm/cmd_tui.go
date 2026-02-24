package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/DohaerisAI/forgevm/tui"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:     "tui",
		Aliases: []string{"ui", "dashboard"},
		Short:   "Launch interactive terminal dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			model := tui.NewModel(serverURL, apiKey)
			p := tea.NewProgram(model, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("tui: %w", err)
			}
			return nil
		},
	}
}
