package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/gcp"
)

// validGcpName guards GCP profile names the same way validAwsName guards AWS
// ones, reserving the gcp global-conf basename.
var validGcpName = newValidName("gcp", "gcp")

func newGcpLoginCmd() *cobra.Command {
	var configName, project, region string
	var isolate, gcpYes, gcpNoLink bool
	var envrc *envrcFlags
	c := &cobra.Command{
		Use:   "login [name]",
		Short: "Sign in to a Google Cloud account (browser pops on your local machine)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prov := gcp.NewProvider()
			dir := prov.ProfilesDir()
			name, newProfile, err := resolveLoginTarget(cmd, prov, args, "azrl gcp", validGcpName)
			if err != nil {
				return err
			}
			if err := validGcpName(name); err != nil {
				return err
			}
			created := false
			conf, err := gcp.LoadConf(name, dir)
			if err != nil {
				cn := configName
				if cn == "" {
					cn = name
				}
				detail := project
				if detail == "" {
					detail = cn
				}
				// newProfile: already committed via the first-login name prompt, so
				// create without a second confirm. Otherwise confirm before creating.
				if !newProfile && !confirmCreateProfile(cmd, "azrl gcp", name, detail, gcpYes) {
					return usageErr("azrl gcp: no profile %q — pass --yes to create it (%s) or run interactively", name, detail)
				}
				conf = gcp.Conf{ConfigName: cn, Project: project, Region: region, Isolate: isolate}
				if werr := conf.Write(groupConfPath(dir, name)); werr != nil {
					return werr
				}
				created = true
				fmt.Fprintf(cmd.ErrOrStderr(), "azrl gcp: created profile %q (%s)\n", name, detail)
			} else if isolate && !conf.Isolate {
				conf.Isolate = true
				if serr := gcp.SetIsolate(dir, name, true); serr != nil {
					return serr
				}
			}
			if err := gcp.SyncConfig(name, conf, conf.Isolate); err != nil {
				return err
			}
			configName := conf.ResolvedConfigName(name)
			fmt.Fprintf(cmd.ErrOrStderr(), "azrl gcp: signing in to project %q as profile %q\n", conf.Project, name)
			if conf.BrowserCmd != "" {
				// gcp.Login loads config.Global itself; the env hook in
				// LoadGlobal picks this up (same pattern as AZURE_CONFIG_DIR).
				os.Setenv("AZRL_BROWSER_CMD", conf.BrowserCmd)
			}
			if _, err := loadGlobalOrSetup(cmd.ErrOrStderr()); err != nil {
				return err
			}
			if err := gcp.Login(dir, name, configName, conf.Isolate); err != nil {
				return err
			}
			if conf.ExpectAccount != "" {
				account, err := gcp.ActiveAccount(dir, name, configName, conf.Isolate)
				if err != nil {
					return fmt.Errorf("azrl gcp: could not verify signed-in account: %w", err)
				}
				if err := gcp.AssertAccount(account, conf.ExpectAccount); err != nil {
					return err
				}
			}
			if warn := gcp.GKEIsolationWarning(conf.Isolate); warn != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), warn)
			}
			pwd, _ := os.Getwd()
			if created && !gcpNoLink {
				// Pin-on-create (all providers): creating = Sign in + Use here in
				// one. Sign-in of an existing profile deliberately never pins.
				if err := prov.Use(name, dir, pwd); err != nil {
					return err
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "azrl gcp: pinned %s/.gcpprofile -> %q\n", pwd, name)
				offerGcpEnvrc(pwd, name, conf.Isolate, envrc.mode(), cmd.ErrOrStderr(), cmd.InOrStdin())
			}
			printApplyHint(cmd, "azrl gcp", "gcp", name)
			return gcp.Scheme().Touch(name, dir, pwd)
		},
	}
	c.Flags().StringVar(&configName, "config-name", "", "gcloud named configuration to drive (defaults to the profile name)")
	c.Flags().StringVar(&project, "project", "", "GCP project to bind")
	c.Flags().StringVar(&region, "region", "", "Default compute region to bind")
	c.Flags().BoolVar(&isolate, "isolate", false, "Scope this profile to its own CLOUDSDK_CONFIG dir")
	c.Flags().BoolVarP(&gcpYes, "yes", "y", false, "Create a missing profile without prompting.")
	c.Flags().BoolVar(&gcpNoLink, "no-map", false, "Create without claiming this directory (skip the .gcpprofile pin).")
	envrc = addEnvrcFlags(c)
	return c
}

func newGcpUseCmd() *cobra.Command {
	var isolate bool
	var envrc *envrcFlags
	c := &cobra.Command{
		Use:   "use <name>",
		Short: "Pin the current directory to a GCP profile and sync its gcloud configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validGcpName(name); err != nil {
				return withCode(ExitUsage, err)
			}
			prov := gcp.NewProvider()
			dir := prov.ProfilesDir()
			if err := requireProfile("azrl gcp", prov, name); err != nil {
				return err
			}
			conf, err := gcp.LoadConf(name, dir)
			if err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			if err := prov.Use(name, dir, pwd); err != nil {
				return err
			}
			if isolate && !conf.Isolate {
				conf.Isolate = true
				if serr := gcp.SetIsolate(dir, name, true); serr != nil {
					return serr
				}
			}
			if err := gcp.SyncConfig(name, conf, conf.Isolate); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "azrl gcp: pinned %s/.gcpprofile -> %q (config sync skipped: %v)\n", pwd, name, err)
				return nil
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "azrl gcp: pinned %s/.gcpprofile -> %q and synced the gcloud configuration\n", pwd, name)
			offerGcpEnvrc(pwd, name, conf.Isolate, envrc.mode(), cmd.ErrOrStderr(), cmd.InOrStdin())
			if warn := gcp.GKEIsolationWarning(conf.Isolate); warn != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), warn)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&isolate, "isolate", false, "Scope this profile to its own CLOUDSDK_CONFIG dir")
	envrc = addEnvrcFlags(c)
	return c
}

func newGcpCaptureCmd() *cobra.Command {
	var configName, project, region, expectAccount string
	var asJSON bool
	c := &cobra.Command{
		Use:   "capture [name]",
		Short: "Record a GCP account into a profile from its project details (metadata only, no mapping)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := captureName(args)
			if err := validGcpName(name); err != nil {
				return withCode(ExitUsage, err)
			}
			stdout := cmd.OutOrStdout()
			if asJSON {
				var restore func()
				stdout, restore = jsonStdout(cmd)
				defer restore()
			}
			prov := gcp.NewProvider()
			dir := prov.ProfilesDir()
			def := gcp.CaptureDefaults()
			cn := configName
			if cn == "" {
				cn = def.ConfigName
			}
			if cn == "" {
				cn = name
			}
			conf := gcp.Conf{
				ConfigName: cn, Project: project, Region: region, ExpectAccount: expectAccount,
			}
			if conf.Project == "" {
				conf.Project = def.Project
			}
			if conf.Region == "" {
				conf.Region = def.Region
			}
			created := true
			if existing, err := gcp.LoadConf(name, dir); err == nil {
				created = false
				conf.Label = existing.Label
				conf.Isolate = existing.Isolate
				conf.BrowserCmd = existing.BrowserCmd
				conf.BrowserLabel = existing.BrowserLabel
			}
			if err := conf.Write(groupConfPath(dir, name)); err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			if err := gcp.Scheme().Touch(name, dir, pwd); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "azrl gcp: captured project %q into profile %q\n", conf.Project, name)
			if asJSON {
				return writeJSON(stdout, captureResult{Provider: "gcp", Profile: name, Identity: conf.ExpectAccount, Detail: conf.Project, Created: created})
			}
			return nil
		},
	}
	c.Flags().StringVar(&configName, "config-name", "", "gcloud named configuration (defaults to the profile name)")
	c.Flags().StringVar(&project, "project", "", "GCP project to bind")
	c.Flags().StringVar(&region, "region", "", "Default compute region")
	c.Flags().StringVar(&expectAccount, "expect-account", "", "Account email to assert after sign-in")
	addJSONFlag(c, &asJSON, "the captured profile")
	return c
}

// offerGcpEnvrc offers to write a direnv .envrc so plain `gcloud` in pwd follows
// the profile. A closed/non-tty stdin reads as a decline (never hangs).
func offerGcpEnvrc(pwd, name string, isolate bool, mode envrcMode, out io.Writer, in io.Reader) {
	offerGroupEnvrc("gcp", "gcloud", gcp.WriteEnvrc, pwd, name, isolate, mode, out, in)
}

// gcpSubcommands builds a fresh set of the GCP subcommands.
func gcpSubcommands() []*cobra.Command {
	return []*cobra.Command{
		newGcpLoginCmd(),
		newGroupListCmd(gcp.NewProvider, "List configured GCP profiles and their projects"),
		newGcpUseCmd(),
		newGroupRmCmd(gcp.NewProvider, "Remove a GCP profile and its isolated config dir", validGcpName),
		newGcpCaptureCmd(),
		newGroupStatusCmd(gcp.NewProvider, "Show the ambient and repo-pinned GCP configurations", ".gcpprofile", "CLOUDSDK_ACTIVE_CONFIG_NAME"),
		newGcpBrowserCmd(),
		newShellCmd("gcp", "Open a subshell acting as a GCP profile (no mapping)"),
		newConsoleCmd("gcp", "Open the GCP console for a profile's project"),
		newUnlinkCmd("gcp", "Remove this directory's GCP profile mapping (keeps the profile)"),
		newEnvCmd(gcp.NewProvider, "azrl gcp", "gcp", validGcpName),
		newDefaultCmd("gcp", "Sign the native gcloud session in as this profile's account via the bridge"),
	}
}

// newGcpGroupCmd is the `gcp` parent for the unified azrl binary.
func newGcpGroupCmd() *cobra.Command {
	g := &cobra.Command{
		Use:   "gcp",
		Short: "Manage Google Cloud accounts (login, use, capture, …)",
	}
	g.AddCommand(gcpSubcommands()...)
	return g
}

func init() { RootCmd.AddCommand(newGcpGroupCmd()) }
