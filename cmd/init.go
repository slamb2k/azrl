package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Tenant-less login, then record the session as a profile",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		g, err := config.LoadGlobal(config.ProfilesDir())
		if err != nil {
			return err
		}
		pwd, _ := os.Getwd()
		arg := ""
		if len(args) == 1 {
			arg = args[0]
		}
		name := profile.DefaultName(arg, pwd)
		confPath := filepath.Join(config.ProfilesDir(), name+".conf")
		if _, err := os.Stat(confPath); err == nil {
			return fmt.Errorf("azrl: %s already exists — remove it first", confPath)
		}
		cfgDir := filepath.Join(config.ProfilesDir(), name)
		os.MkdirAll(cfgDir, 0o755)
		os.Setenv("AZURE_CONFIG_DIR", cfgDir)
		cmd.Printf("azrl: init profile=%s (tenant-less sign-in)\n", name)
		azure.CleanSlate(cfgDir)
		if err := runLogin("", g, false, cmd.OutOrStdout()); err != nil {
			return err
		}
		return captureSession(name, pwd, cmd.OutOrStdout())
	},
}

func init() { RootCmd.AddCommand(initCmd) }
