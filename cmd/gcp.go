package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/gcp"
)

// validGcpName guards GCP profile names the same way validAwsName guards AWS
// ones, reserving the gcp global-conf basename.
func validGcpName(name string) error {
	if name == "" {
		return fmt.Errorf("gcp: a profile name is required")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("gcp: invalid profile name %q", name)
	}
	if name == "gcp" {
		return fmt.Errorf("gcp: refusing to use the global gcp config")
	}
	return nil
}

// gcpConfPath is the conf file path for a GCP profile.
func gcpConfPath(dir, name string) string {
	return dir + string(os.PathSeparator) + name + ".conf"
}

func newGcpLoginCmd() *cobra.Command {
	var configName, project, region string
	var isolate, gcpYes, gcpNoLink bool
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
					return fmt.Errorf("azrl gcp: no profile %q — pass --yes to create it (%s) or run interactively", name, detail)
				}
				conf = gcp.Conf{ConfigName: cn, Project: project, Region: region, Isolate: isolate}
				if werr := conf.Write(gcpConfPath(dir, name)); werr != nil {
					return werr
				}
				created = true
				cmd.Printf("azrl gcp: created profile %q (%s)\n", name, detail)
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
			cmd.Printf("gcp: signing in to project %q as profile %q\n", conf.Project, name)
			if conf.BrowserCmd != "" {
				// gcp.Login loads config.Global itself; the env hook in
				// LoadGlobal picks this up (same pattern as AZURE_CONFIG_DIR).
				os.Setenv("AZRL_BROWSER_CMD", conf.BrowserCmd)
			}
			if _, err := loadGlobalOrSetup(cmd.OutOrStdout()); err != nil {
				return err
			}
			if err := gcp.Login(dir, name, configName, conf.Isolate); err != nil {
				return err
			}
			if conf.ExpectAccount != "" {
				account, err := gcp.ActiveAccount(dir, name, configName, conf.Isolate)
				if err != nil {
					return fmt.Errorf("gcp: could not verify signed-in account: %w", err)
				}
				if err := gcp.AssertAccount(account, conf.ExpectAccount); err != nil {
					return err
				}
			}
			if warn := gcp.GKEIsolationWarning(conf.Isolate); warn != "" {
				cmd.Println(warn)
			}
			pwd, _ := os.Getwd()
			if created && !gcpNoLink {
				// Pin-on-create (all providers): creating = Sign in + Use here in
				// one. Sign-in of an existing profile deliberately never pins.
				if err := prov.Use(name, dir, pwd); err != nil {
					return err
				}
				cmd.Printf("gcp: pinned %s/.gcpprofile -> %q\n", pwd, name)
				offerGcpEnvrc(pwd, name, conf.Isolate, cmd.OutOrStdout(), cmd.InOrStdin())
			}
			return gcp.Scheme().Touch(name, dir, pwd)
		},
	}
	c.Flags().StringVar(&configName, "config-name", "", "gcloud named configuration to drive (defaults to the profile name)")
	c.Flags().StringVar(&project, "project", "", "GCP project to bind")
	c.Flags().StringVar(&region, "region", "", "Default compute region to bind")
	c.Flags().BoolVar(&isolate, "isolate", false, "Scope this profile to its own CLOUDSDK_CONFIG dir")
	c.Flags().BoolVarP(&gcpYes, "yes", "y", false, "Create a missing profile without prompting.")
	c.Flags().BoolVar(&gcpNoLink, "no-map", false, "Create without claiming this directory (skip the .gcpprofile pin).")
	c.Flags().SetNormalizeFunc(normalizeLegacyFlags)
	return c
}

func newGcpListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured GCP profiles and their projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			prov := gcp.NewProvider()
			profs, err := prov.ListProfiles(prov.ProfilesDir())
			if err != nil {
				return err
			}
			pairs := make([][2]string, len(profs))
			for i, p := range profs {
				pairs[i] = [2]string{p.Display(), p.Detail}
			}
			printList(cmd.OutOrStdout(), pairs)
			return nil
		},
	}
}

func newGcpUseCmd() *cobra.Command {
	var isolate bool
	c := &cobra.Command{
		Use:   "use <name>",
		Short: "Pin the current directory to a GCP profile and sync its gcloud configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validGcpName(name); err != nil {
				return err
			}
			prov := gcp.NewProvider()
			dir := prov.ProfilesDir()
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
				cmd.Printf("gcp: pinned %s/.gcpprofile -> %q (config sync skipped: %v)\n", pwd, name, err)
				return nil
			}
			cmd.Printf("gcp: pinned %s/.gcpprofile -> %q and synced the gcloud configuration\n", pwd, name)
			offerGcpEnvrc(pwd, name, conf.Isolate, cmd.OutOrStdout(), cmd.InOrStdin())
			if warn := gcp.GKEIsolationWarning(conf.Isolate); warn != "" {
				cmd.Println(warn)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&isolate, "isolate", false, "Scope this profile to its own CLOUDSDK_CONFIG dir")
	return c
}

func newGcpRmCmd() *cobra.Command {
	var unlinkAll bool
	var replace string
	c := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a GCP profile and its isolated config dir",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validGcpName(name); err != nil {
				return err
			}
			prov := gcp.NewProvider()
			dir := prov.ProfilesDir()
			if err := refuseIfLinked(prov.Scheme(), dir, name, unlinkAll, replace); err != nil {
				return err
			}
			if err := unlinkOrReplace(cmd, prov.Scheme(), dir, name, unlinkAll, replace); err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			removed, err := prov.Remove(name, dir, pwd)
			if err != nil {
				return err
			}
			for _, r := range removed {
				cmd.Printf("removed %s\n", r)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&unlinkAll, "unmap-all", false, "Remove every directory mapping before deleting the profile")
	c.Flags().StringVar(&replace, "replace", "", "Repoint every directory link at this profile before deleting")
	c.Flags().SetNormalizeFunc(normalizeLegacyFlags)
	c.MarkFlagsMutuallyExclusive("unmap-all", "replace")
	return c
}

func newGcpCaptureCmd() *cobra.Command {
	var configName, project, region, expectAccount string
	c := &cobra.Command{
		Use:   "capture <name>",
		Short: "Record a GCP account into a profile from its project details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validGcpName(name); err != nil {
				return err
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
			if existing, err := gcp.LoadConf(name, dir); err == nil {
				conf.Label = existing.Label
				conf.Isolate = existing.Isolate
				conf.BrowserCmd = existing.BrowserCmd
				conf.BrowserLabel = existing.BrowserLabel
			}
			if err := conf.Write(gcpConfPath(dir, name)); err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			if err := gcp.Scheme().Touch(name, dir, pwd); err != nil {
				return err
			}
			cmd.Printf("gcp: captured project %q into profile %q\n", project, name)
			return nil
		},
	}
	c.Flags().StringVar(&configName, "config-name", "", "gcloud named configuration (defaults to the profile name)")
	c.Flags().StringVar(&project, "project", "", "GCP project to bind")
	c.Flags().StringVar(&region, "region", "", "Default compute region")
	c.Flags().StringVar(&expectAccount, "expect-account", "", "Account email to assert after sign-in")
	return c
}

func newGcpStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the ambient and repo-pinned GCP configurations",
		RunE: func(cmd *cobra.Command, args []string) error {
			if p := os.Getenv("CLOUDSDK_ACTIVE_CONFIG_NAME"); p != "" {
				cmd.Printf("ambient CLOUDSDK_ACTIVE_CONFIG_NAME: %s\n", p)
			} else {
				cmd.Println("ambient CLOUDSDK_ACTIVE_CONFIG_NAME: (unset)")
			}
			prov := gcp.NewProvider()
			pwd, _ := os.Getwd()
			if pin, err := prov.Resolve("", pwd); err == nil {
				cmd.Printf("this dir is pinned to: %s\n", pin)
			} else {
				cmd.Println("this dir has no .gcpprofile pin")
			}
			return nil
		},
	}
}

// offerGcpEnvrc offers to write a direnv .envrc so plain `gcloud` in pwd follows
// the profile. A closed/non-tty stdin reads as a decline (never hangs).
func offerGcpEnvrc(pwd, name string, isolate bool, out io.Writer, in io.Reader) {
	fmt.Fprint(out, "gcp: also write .envrc so `gcloud` in this dir follows this profile? [y/N] ")
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		fmt.Fprintln(out)
		return
	}
	if ans := strings.TrimSpace(sc.Text()); !strings.HasPrefix(strings.ToLower(ans), "y") {
		return
	}
	wrote, err := gcp.WriteEnvrc(pwd, name, isolate)
	if err != nil {
		fmt.Fprintf(out, "gcp: could not write .envrc: %v\n", err)
		return
	}
	if wrote {
		fmt.Fprintf(out, "gcp: wrote %s/.envrc — run `direnv allow` to activate\n", pwd)
	}
}

// gcpSubcommands builds a fresh set of the GCP subcommands.
func gcpSubcommands() []*cobra.Command {
	return []*cobra.Command{
		newGcpLoginCmd(), newGcpListCmd(), newGcpUseCmd(),
		newGcpRmCmd(), newGcpCaptureCmd(), newGcpStatusCmd(), newGcpBrowserCmd(),
		newShellCmd("gcp", "Open a subshell acting as a GCP profile (no mapping)"),
		newConsoleCmd("gcp", "Open the GCP console for a profile's project"),
		newUnlinkCmd("gcp", "Remove this directory's GCP profile mapping (keeps the profile)"),
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
