package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/github"
)

// validGhName guards GitHub profile names the same way validAzureName guards
// Azure ones, reserving the ghrl global-conf basename.
var validGhName = newValidName("ghrl", "ghrl")

func newGhLoginCmd() *cobra.Command {
	var hostname string
	var ghYes, ghNoLink bool
	var envrc *envrcFlags
	var res loginResult
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
					return usageErr("ghrl: no profile %q — pass --yes to create it (host %s) or run interactively", name, hostname)
				}
				conf = github.Conf{Host: hostname, Protocol: "https"}
				if werr := conf.Write(groupConfPath(dir, name)); werr != nil {
					return werr
				}
				created = true
				fmt.Fprintf(cmd.ErrOrStderr(), "ghrl: created profile %q (%s)\n", name, hostname)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "ghrl: signing in to %s as profile %q\n", conf.Host, name)
			if _, err := loadGlobalOrSetup(cmd.ErrOrStderr()); err != nil {
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
					fmt.Fprintf(cmd.ErrOrStderr(), "ghrl: pinned %s/.ghprofile -> %q (credential wiring skipped: %v)\n", pwd, name, err)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "ghrl: pinned %s/.ghprofile -> %q and wired git-HTTPS for %s\n", pwd, name, conf.Host)
				}
				offerGhEnvrc(pwd, name, envrc.mode(), cmd.ErrOrStderr(), cmd.InOrStdin())
			}
			printApplyHint(cmd, "ghrl", "github", name)
			if err := github.Scheme().Touch(name, dir, pwd); err != nil {
				return err
			}
			res.Profile, res.Created = name, created
			return nil
		},
	}
	c.Flags().StringVar(&hostname, "hostname", "github.com", "GitHub host (github.com, a *.ghe.com tenant, or a GHES hostname)")
	c.Flags().BoolVarP(&ghYes, "yes", "y", false, "Create a missing profile without prompting.")
	c.Flags().BoolVar(&ghNoLink, "no-map", false, "Create without claiming this directory (skip the .ghprofile pin).")
	envrc = addEnvrcFlags(c)
	addLoginJSON(c, "github", &res)
	return c
}

func newGhUseCmd() *cobra.Command {
	var envrc *envrcFlags
	c := &cobra.Command{
		Use:   "use <name>",
		Short: "Pin the current directory to a GitHub profile and wire git-HTTPS",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validGhName(name); err != nil {
				return withCode(ExitUsage, err)
			}
			prov := github.NewProvider()
			dir := prov.ProfilesDir()
			if err := requireProfile("ghrl", prov, name); err != nil {
				return err
			}
			conf, err := github.LoadConf(name, dir)
			if err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			if err := prov.Use(name, dir, pwd); err != nil {
				return err
			}
			if err := github.SetupRepo(dir, name, pwd, conf); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "ghrl: pinned %s/.ghprofile -> %q (credential wiring skipped: %v)\n", pwd, name, err)
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "ghrl: pinned %s/.ghprofile -> %q and wired git-HTTPS for %s\n", pwd, name, conf.Host)
			}
			offerGhEnvrc(pwd, name, envrc.mode(), cmd.ErrOrStderr(), cmd.InOrStdin())
			return nil
		},
	}
	envrc = addEnvrcFlags(c)
	return c
}

// offerGhEnvrc offers to write a direnv .envrc so plain `gh` in pwd follows the
// profile (GH_CONFIG_DIR). mode decides whether it asks, writes, or stays quiet.
func offerGhEnvrc(pwd, name string, mode envrcMode, out io.Writer, in io.Reader) {
	offerGroupEnvrc("ghrl", "gh", github.WriteEnvrc, pwd, name, false, mode, out, in)
}

func newGhCaptureCmd() *cobra.Command {
	var hostname string
	var asJSON bool
	c := &cobra.Command{
		Use:   "capture [name]",
		Short: "Record the currently signed-in gh session into a profile (metadata only, no mapping)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := captureName(args)
			if err := validGhName(name); err != nil {
				return withCode(ExitUsage, err)
			}
			stdout := cmd.OutOrStdout()
			if asJSON {
				var restore func()
				stdout, restore = jsonStdout(cmd)
				defer restore()
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
				return needsHumanErr("ghrl: no gh session to capture on %s — sign in first (%v)", hostname, err)
			}
			conf := github.Conf{Host: hostname, User: login, Protocol: "https"}
			created := true
			if existing, err := github.LoadConf(name, dir); err == nil {
				created = false
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
			fmt.Fprintf(cmd.ErrOrStderr(), "ghrl: captured %s@%s into profile %q\n", login, hostname, name)
			if asJSON {
				return writeJSON(stdout, captureResult{Provider: "github", Profile: name, Identity: login, Detail: hostname, Created: created})
			}
			return nil
		},
	}
	c.Flags().StringVar(&hostname, "hostname", "github.com", "GitHub host to record")
	addJSONFlag(c, &asJSON, "the captured profile")
	return c
}

// githubSubcommands builds a fresh set of the GitHub subcommands (cobra commands
// bind to one parent, so both `azrl gh …` and the ghrl alias build their own).
func githubSubcommands() []*cobra.Command {
	return []*cobra.Command{
		newGhLoginCmd(),
		newGhUseCmd(), newGroupRmCmd(github.NewProvider, "Remove a GitHub profile and its isolated config dir", validGhName),
		newGhCaptureCmd(),
		newGhBrowserCmd(),
		newShellCmd("github", "Open a subshell acting as a GitHub profile (no mapping)"),
		newConsoleCmd("github", "Open GitHub as a profile's account"),
		newUnlinkCmd("github", "Remove this directory's GitHub profile mapping (keeps the profile)"),
		newEnvCmd(github.NewProvider, "ghrl", "github", validGhName),
		newDefaultCmd("github", "Make a profile's account the native gh default (web login via the bridge if needed)"),
	}
}

// newGhGroupCmd is the `gh` parent for the unified azrl binary.
func newGhGroupCmd() *cobra.Command {
	g := &cobra.Command{
		Use:   "gh",
		Short: "Manage GitHub accounts (login, use, capture, …)",
		RunE:  groupRun,
	}
	g.AddCommand(githubSubcommands()...)
	return g
}

func init() { RootCmd.AddCommand(newGhGroupCmd()) }
