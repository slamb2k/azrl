package cmd

import (
	"os"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var captureJSON bool

var captureCmd = &cobra.Command{
	Use:   "capture [name]",
	Short: "Record the current az session as a profile (metadata only, no login, no mapping)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pwd, _ := os.Getwd()
		arg := ""
		if len(args) == 1 {
			arg = args[0]
		}
		name := profile.DefaultName(arg, pwd)
		if err := validProfileName(name); err != nil {
			return withCode(ExitUsage, err)
		}
		if captureJSON {
			stdout, restore := jsonStdout(cmd)
			defer restore()
			c, created, err := captureSession(name, pwd, false, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			return writeJSON(stdout, captureResult{Provider: "azure", Profile: name, Identity: c.ExpectUser, Detail: c.Tenant, Created: created})
		}
		_, _, err := captureSession(name, pwd, false, cmd.OutOrStdout())
		return err
	},
}

func init() {
	addJSONFlag(captureCmd, &captureJSON, "the captured profile")
	RootCmd.AddCommand(captureCmd)
}
