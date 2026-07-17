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
	// Runtime errors (e.g. an unresolved login target) shouldn't dump the full
	// usage block; cobra inherits this to subcommands. Errors are still printed.
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := loadGlobalOrSetup(cmd.OutOrStdout()); err != nil {
			return err
		}
		return ui.Run()
	},
}

// Execute runs the root command with help grouped into the everyday core
// and the power-user shelf, so `azrl --help` teaches the product in four
// verbs instead of fourteen.
func Execute() error {
	groupCommands(RootCmd)
	return RootCmd.Execute()
}

// groupCommands sorts the root's visible commands into help groups: the core
// loop (sign in, point a directory, ask who governs here), providers, and
// everything else under Advanced. Unknown/new commands land in Advanced by
// default — the core set is a deliberate, short list.
func groupCommands(root *cobra.Command) {
	root.AddGroup(
		&cobra.Group{ID: "core", Title: "Core Commands:"},
		&cobra.Group{ID: "providers", Title: "Provider Groups:"},
		&cobra.Group{ID: "advanced", Title: "Advanced Commands:"},
	)
	core := map[string]bool{"login": true, "use": true, "whoami": true, "rm": true, "setup": true}
	providers := map[string]bool{"gh": true, "aws": true, "gcp": true}
	for _, c := range root.Commands() {
		switch {
		case core[c.Name()]:
			c.GroupID = "core"
		case providers[c.Name()]:
			c.GroupID = "providers"
		case c.Hidden || c.Name() == "help" || c.Name() == "completion":
			// leave ungrouped
		default:
			c.GroupID = "advanced"
		}
	}
}
