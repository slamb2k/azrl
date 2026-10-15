package cmd

import (
	"os"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var useEnvrc *envrcFlags

var useCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Link the current directory to an existing profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := validProfileName(name); err != nil {
			return withCode(ExitUsage, err)
		}
		if err := requireProfile("azrl", azure.NewProvider(), name); err != nil {
			return err
		}
		pwd, _ := os.Getwd()
		if err := profile.Use(name, config.ProfilesDir(), pwd); err != nil {
			return err
		}
		cmd.Printf("azrl: linked %s/.azprofile -> profile %q\n", pwd, name)
		offerEnvrc(pwd, useEnvrc.mode(), cmd.OutOrStdout(), cmd.InOrStdin())
		return nil
	},
}

func init() {
	useEnvrc = addEnvrcFlags(useCmd)
	RootCmd.AddCommand(useCmd)
}
