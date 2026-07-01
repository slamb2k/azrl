package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/github"
)

// validGhName guards GitHub profile names the same way validProfileName guards
// Azure ones, reserving the ghrl global-conf basename.
func validGhName(name string) error {
	if name == "" {
		return fmt.Errorf("ghrl: a profile name is required")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("ghrl: invalid profile name %q", name)
	}
	if name == "ghrl" {
		return fmt.Errorf("ghrl: refusing to use the global ghrl config")
	}
	return nil
}

func newGhLoginCmd() *cobra.Command {
	var hostname string
	c := &cobra.Command{
		Use:   "login [name]",
		Short: "Sign in to a GitHub account (browser pops on your local machine)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prov := github.NewProvider()
			dir := prov.ProfilesDir()
			name := ""
			if len(args) == 1 {
				name = args[0]
			} else {
				pwd, _ := os.Getwd()
				name, _ = prov.Resolve("", pwd)
			}
			if err := validGhName(name); err != nil {
				return err
			}
			conf, err := github.LoadConf(name, dir)
			if err != nil {
				// New profile: seed a conf from the hostname flag.
				conf = github.Conf{Host: hostname, Protocol: "https"}
				if werr := conf.Write(ghConfPath(dir, name)); werr != nil {
					return werr
				}
			}
			cmd.Printf("ghrl: signing in to %s as profile %q\n", conf.Host, name)
			return github.Login(dir, name, conf)
		},
	}
	c.Flags().StringVar(&hostname, "hostname", "github.com", "GitHub host (github.com, a *.ghe.com tenant, or a GHES hostname)")
	return c
}

func newGhListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured GitHub profiles and their hosts",
		RunE: func(cmd *cobra.Command, args []string) error {
			prov := github.NewProvider()
			profs, err := prov.ListProfiles(prov.ProfilesDir())
			if err != nil {
				return err
			}
			for _, p := range profs {
				cmd.Printf("%-24s %s\n", p.Display(), p.Detail)
			}
			return nil
		},
	}
}

func newGhUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Pin the current directory to a GitHub profile and wire git-HTTPS",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validGhName(name); err != nil {
				return err
			}
			prov := github.NewProvider()
			dir := prov.ProfilesDir()
			conf, err := github.LoadConf(name, dir)
			if err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			if err := prov.Use(name, dir, pwd); err != nil {
				return err
			}
			if err := github.SetupRepo(dir, name, pwd, conf); err != nil {
				cmd.Printf("ghrl: pinned %s/.ghprofile -> %q (credential wiring skipped: %v)\n", pwd, name, err)
				return nil
			}
			cmd.Printf("ghrl: pinned %s/.ghprofile -> %q and wired git-HTTPS for %s\n", pwd, name, conf.Host)
			return nil
		},
	}
}

func newGhSwitchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <name>",
		Short: "Set the active GitHub profile (default when a repo has no .ghprofile)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validGhName(name); err != nil {
				return err
			}
			prov := github.NewProvider()
			if err := github.Switch(prov.ProfilesDir(), name); err != nil {
				return err
			}
			cmd.Printf("ghrl: active profile is now %q\n", name)
			return nil
		},
	}
}

func newGhRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a GitHub profile and its isolated config dir",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validGhName(name); err != nil {
				return err
			}
			prov := github.NewProvider()
			pwd, _ := os.Getwd()
			removed, err := prov.Remove(name, prov.ProfilesDir(), pwd)
			if err != nil {
				return err
			}
			for _, r := range removed {
				cmd.Printf("removed %s\n", r)
			}
			return nil
		},
	}
}

func newGhCaptureCmd() *cobra.Command {
	var hostname string
	c := &cobra.Command{
		Use:   "capture <name>",
		Short: "Record the currently signed-in gh session into a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validGhName(name); err != nil {
				return err
			}
			prov := github.NewProvider()
			dir := prov.ProfilesDir()
			login, err := github.WhoAmI(dir, name, hostname)
			if err != nil {
				return err
			}
			conf := github.Conf{Host: hostname, User: login, Protocol: "https"}
			if err := conf.Write(ghConfPath(dir, name)); err != nil {
				return err
			}
			cmd.Printf("ghrl: captured %s@%s into profile %q\n", login, hostname, name)
			return nil
		},
	}
	c.Flags().StringVar(&hostname, "hostname", "github.com", "GitHub host to record")
	return c
}

func newGhStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active and repo-pinned GitHub profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			prov := github.NewProvider()
			dir := prov.ProfilesDir()
			if cur := github.Current(dir); cur != "" {
				cmd.Printf("active profile: %s\n", cur)
			} else {
				cmd.Println("active profile: (none — use `gh switch`)")
			}
			pwd, _ := os.Getwd()
			if pin, err := prov.Resolve("", pwd); err == nil {
				cmd.Printf("this dir is pinned to: %s\n", pin)
			} else {
				cmd.Println("this dir has no .ghprofile pin")
			}
			return nil
		},
	}
}

// githubSubcommands builds a fresh set of the GitHub subcommands (cobra commands
// bind to one parent, so both `azrl gh …` and the ghrl alias build their own).
func githubSubcommands() []*cobra.Command {
	return []*cobra.Command{
		newGhLoginCmd(), newGhListCmd(), newGhUseCmd(), newGhSwitchCmd(),
		newGhRmCmd(), newGhCaptureCmd(), newGhStatusCmd(),
	}
}

// newGhGroupCmd is the `gh` parent for the unified azrl binary.
func newGhGroupCmd() *cobra.Command {
	g := &cobra.Command{
		Use:   "gh",
		Short: "Manage GitHub accounts (login, use, switch, …)",
	}
	g.AddCommand(githubSubcommands()...)
	return g
}

// ghConfPath is the conf file path for a GitHub profile.
func ghConfPath(dir, name string) string {
	return dir + string(os.PathSeparator) + name + ".conf"
}

func init() { RootCmd.AddCommand(newGhGroupCmd()) }
