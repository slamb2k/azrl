package cmd

import (
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured profiles and their tenants",
	RunE: func(cmd *cobra.Command, args []string) error {
		profs, err := profile.List(config.ProfilesDir())
		if err != nil {
			return err
		}
		for _, p := range profs {
			cmd.Printf("%-24s %s\n", p.Name, p.Tenant)
		}
		return nil
	},
}

func init() { RootCmd.AddCommand(listCmd) }
