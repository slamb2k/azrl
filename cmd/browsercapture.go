package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var browserCaptureCmd = &cobra.Command{
	Use:    "__browser-capture [url]",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		capfile := os.Getenv("AZRL_CAPFILE")
		if capfile == "" {
			return fmt.Errorf("AZRL_CAPFILE not set")
		}
		return os.WriteFile(capfile, []byte(args[0]), 0o600)
	},
}

func init() {
	RootCmd.AddCommand(browserCaptureCmd)
}
