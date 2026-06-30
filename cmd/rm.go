package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var rmYes bool

var rmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove a profile: its conf, token dir, and matching .azprofile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := validProfileName(name); err != nil {
			return err
		}
		pwd, _ := os.Getwd()
		confdir := config.ProfilesDir()
		targets := profile.RemoveTargets(name, confdir, pwd)
		if len(targets) == 0 {
			cmd.Printf("azrl: nothing to remove for %q\n", name)
			return nil
		}
		cmd.Println("azrl: will remove:")
		for _, t := range targets {
			cmd.Printf("  %s\n", t)
		}
		if !rmYes {
			cmd.Print("Remove these? [y/N] ")
			sc := bufio.NewScanner(os.Stdin)
			sc.Scan()
			if ans := strings.TrimSpace(sc.Text()); !strings.HasPrefix(strings.ToLower(ans), "y") {
				return fmt.Errorf("azrl: aborted")
			}
		}
		if _, err := profile.Remove(name, confdir, pwd); err != nil {
			return err
		}
		cmd.Printf("azrl: removed profile %q\n", name)
		return nil
	},
}

// validProfileName rejects empty names, names containing '/', and the reserved
// global config name.
func validProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("azrl: a profile name is required")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("azrl: invalid profile name %q", name)
	}
	if name == "azrl" {
		return fmt.Errorf("azrl: refusing to use the global azrl config")
	}
	return nil
}

func init() {
	rmCmd.Flags().BoolVarP(&rmYes, "yes", "y", false, "Skip the confirmation prompt")
	RootCmd.AddCommand(rmCmd)
}
