package cmd

import (
	"os"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var captureCmd = &cobra.Command{
	Use:   "capture [name]",
	Short: "Record the current az session as a profile (no login)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pwd, _ := os.Getwd()
		arg := ""
		if len(args) == 1 {
			arg = args[0]
		}
		name := profile.DefaultName(arg, pwd)
		if err := captureSession(name, pwd, cmd.OutOrStdout()); err != nil {
			return err
		}
		offerEnvrc(pwd, cmd.OutOrStdout(), os.Stdin)
		return nil
	},
}

func init() { RootCmd.AddCommand(captureCmd) }
