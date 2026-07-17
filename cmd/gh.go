package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/github"
)

// validGhName guards GitHub profile names the same way validProfileName guards
// Azure ones, reserving the ghrl global-conf basename.
var validGhName = newValidName("ghrl", "ghrl")

func newGhLoginCmd() *cobra.Command {
	var hostname string
	var ghYes, ghNoLink bool
	c := &cobra.Command{
		Use:   "login [name]",
		Short: "Sign in to a GitHub account (browser pops on your local machine)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prov := github.NewProvider()
			dir := prov.ProfilesDir()
			name, newProfile, err := resolveLoginTarget(cmd, prov, args, "ghrl", validGhName)
			if err != nil {
				return err
			}
			if err := validGhName(name); err != nil {
				return err
			}
			created := false
			conf, err := github.LoadConf(name, dir)
			if err != nil {
				// newProfile: already committed via the first-login name prompt, so
				// create without a second confirm. Otherwise confirm before creating.
				if !newProfile && !confirmCreateProfile(cmd, "ghrl", name, hostname, ghYes) {
					return fmt.Errorf("ghrl: no profile %q — pass --yes to create it (host %s) or run interactively", name, hostname)
				}
				conf = github.Conf{Host: hostname, Protocol: "https"}
				if werr := conf.Write(groupConfPath(dir, name)); werr != nil {
					return werr
				}
				created = true
				cmd.Printf("ghrl: created profile %q (%s)\n", name, hostname)
			}
			cmd.Printf("ghrl: signing in to %s as profile %q\n", conf.Host, name)
			if _, err := loadGlobalOrSetup(cmd.OutOrStdout()); err != nil {
				return err
			}
			if err := github.Login(dir, name, conf); err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			if created && !ghNoLink {
				// Pin-on-create (all providers): creating = Sign in + Use here in
				// one. Sign-in of an existing profile deliberately never pins.
				if err := prov.Use(name, dir, pwd); err != nil {
					return err
				}
				if err := github.SetupRepo(dir, name, pwd, conf); err != nil {
					cmd.Printf("ghrl: pinned %s/.ghprofile -> %q (credential wiring skipped: %v)\n", pwd, name, err)
				} else {
					cmd.Printf("ghrl: pinned %s/.ghprofile -> %q and wired git-HTTPS for %s\n", pwd, name, conf.Host)
				}
			}
			return github.Scheme().Touch(name, dir, pwd)
		},
	}
	c.Flags().StringVar(&hostname, "hostname", "github.com", "GitHub host (github.com, a *.ghe.com tenant, or a GHES hostname)")
	c.Flags().BoolVarP(&ghYes, "yes", "y", false, "Create a missing profile without prompting.")
	c.Flags().BoolVar(&ghNoLink, "no-map", false, "Create without claiming this directory (skip the .ghprofile pin).")
	return c
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
			if err != nil && !github.HasSession(dir, name) {
				// Fresh adopt: the isolated dir has no session yet — record the
				// ambient identity instead (capture is metadata-only; sign-in
				// into the isolated dir happens later via `s`).
				login, err = github.AmbientWhoAmI(hostname)
			}
			if err != nil {
				return err
			}
			conf := github.Conf{Host: hostname, User: login, Protocol: "https"}
			if existing, err := github.LoadConf(name, dir); err == nil {
				conf.Label = existing.Label
				conf.BrowserCmd = existing.BrowserCmd
				conf.BrowserLabel = existing.BrowserLabel
			}
			if err := conf.Write(groupConfPath(dir, name)); err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			if err := github.Scheme().Touch(name, dir, pwd); err != nil {
				return err
			}
			cmd.Printf("ghrl: captured %s@%s into profile %q\n", login, hostname, name)
			return nil
		},
	}
	c.Flags().StringVar(&hostname, "hostname", "github.com", "GitHub host to record")
	return c
}

// githubSubcommands builds a fresh set of the GitHub subcommands (cobra commands
// bind to one parent, so both `azrl gh …` and the ghrl alias build their own).
func githubSubcommands() []*cobra.Command {
	return []*cobra.Command{
		newGhLoginCmd(),
		newGroupListCmd(github.NewProvider, "List configured GitHub profiles and their hosts"),
		newGhUseCmd(), newGroupRmCmd(github.NewProvider, "Remove a GitHub profile and its isolated config dir", validGhName),
		newGhCaptureCmd(),
		newGroupStatusCmd(github.NewProvider, "Show the repo-pinned GitHub profile", ".ghprofile", ""),
		newGhBrowserCmd(),
		newShellCmd("github", "Open a subshell acting as a GitHub profile (no mapping)"),
		newConsoleCmd("github", "Open GitHub as a profile's account"),
		newUnlinkCmd("github", "Remove this directory's GitHub profile mapping (keeps the profile)"),
		newDefaultCmd("github", "Make a profile's account the native gh default (web login via the bridge if needed)"),
	}
}

// newGhGroupCmd is the `gh` parent for the unified azrl binary.
func newGhGroupCmd() *cobra.Command {
	g := &cobra.Command{
		Use:   "gh",
		Short: "Manage GitHub accounts (login, use, …)",
	}
	g.AddCommand(githubSubcommands()...)
	return g
}

func init() { RootCmd.AddCommand(newGhGroupCmd()) }
