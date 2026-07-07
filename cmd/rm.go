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
var rmUnlinkAll bool
var rmReplace string

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
		scheme := profile.AzureScheme()
		if err := unlinkOrRefuse(cmd, scheme, confdir, name, rmUnlinkAll, rmReplace); err != nil {
			return err
		}
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

// unlinkOrRefuse handles a rm command's link-awareness: with no linked dirs
// it's a no-op; with links and neither flag it refuses, listing each dir and
// both flags; --unlinkAll removes every link first; --replace repoints every
// link at the given profile first (erroring if that profile doesn't exist).
func unlinkOrRefuse(cmd *cobra.Command, scheme profile.Scheme, confdir, name string, unlinkAll bool, replace string) error {
	dirs := scheme.LinkedDirs(confdir, name)
	if len(dirs) == 0 {
		return nil
	}
	switch {
	case unlinkAll:
		_, err := scheme.UnlinkAll(confdir, name)
		return err
	case replace != "":
		_, err := scheme.ReplaceLinks(confdir, name, replace)
		return err
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%s: profile %q is linked from:\n", scheme.Prefix, name)
		for _, d := range dirs {
			fmt.Fprintf(&b, "  %s\n", d)
		}
		b.WriteString("use --unlink-all to remove the links, or --replace <profile> to repoint them")
		return fmt.Errorf("%s", b.String())
	}
}

func init() {
	rmCmd.Flags().BoolVarP(&rmYes, "yes", "y", false, "Skip the confirmation prompt")
	rmCmd.Flags().BoolVar(&rmUnlinkAll, "unlink-all", false, "Remove every directory link before deleting the profile")
	rmCmd.Flags().StringVar(&rmReplace, "replace", "", "Repoint every directory link at this profile before deleting")
	RootCmd.AddCommand(rmCmd)
}
