package cmd

import (
	"github.com/slamb2k/azrl/internal/ui"
	"github.com/spf13/cobra"
)

// Version is the azrl version string. Overridden at release time via
// -ldflags "-X github.com/slamb2k/azrl/cmd.Version=<tag>".
var Version = "0.2.0"

// RootCmd is the base command. With no subcommand it launches the TUI.
var RootCmd = &cobra.Command{
	Use:     "azrl",
	Short:   "Azure Remote Login — interactive az login from a headless VM",
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ui.Run()
	},
}

// Execute runs the root command.
func Execute() error {
	return RootCmd.Execute()
}
