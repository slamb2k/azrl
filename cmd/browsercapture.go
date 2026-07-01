package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/browsercapture"
)

func newBrowserCaptureCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "__browser-capture [url]",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return browsercapture.Capture(os.Getenv("AZRL_CAPFILE"), args[0])
		},
	}
}

func init() {
	RootCmd.AddCommand(newBrowserCaptureCmd())
}
