package cmd

import (
	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/ui"
)

// GhrlRoot builds the `ghrl` alias entrypoint: a root that preselects the GitHub
// tab on bare invocation and promotes the GitHub subcommands to the top level
// (so `ghrl login` mirrors `azrl gh login`). The hidden browser shims ride along
// because gh/GCM invoke this same binary as their BROWSER.
func GhrlRoot() *cobra.Command {
	root := &cobra.Command{
		Use:     "ghrl",
		Short:   "GitHub Remote Login — interactive gh sign-in from a headless VM",
		Version: Version,
		// Match RootCmd: runtime errors don't dump usage (inherited to subcommands).
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ui.RunGitHub()
		},
	}
	root.AddCommand(githubSubcommands()...)
	root.AddCommand(newBrowserShimCmd())
	root.AddCommand(newBrowserCaptureCmd())
	return root
}

// ExecuteGhrl runs the ghrl alias root.
func ExecuteGhrl() error {
	return GhrlRoot().Execute()
}
