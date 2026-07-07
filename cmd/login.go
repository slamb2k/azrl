package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var loginPaste bool
var loginYes bool
var loginNoLink bool

// validAzureName guards a name typed at the azure first-login prompt: it rejects
// an empty name and any name containing a path separator (azure profiles map to
// a conf file and an isolated config dir named after the profile).
func validAzureName(name string) error {
	if name == "" {
		return fmt.Errorf("azrl: a profile name is required")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("azrl: invalid profile name %q", name)
	}
	return nil
}

var loginCmd = &cobra.Command{
	Use:   "login [profile]",
	Short: "Sign in via the remote-browser bridge",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		g, err := loadGlobalOrSetup(out)
		if err != nil {
			return err
		}
		pwd, _ := os.Getwd()
		prov := azure.NewProvider()
		name, profs, newProfile, rErr := resolveLoginTargetWithProfiles(cmd, prov, args, "azrl", validAzureName)
		if rErr != nil {
			// Preserve azure's tenant-less fallback: with no arg, no directory
			// pin and no saved profiles, sign in to the default ~/.azure instead
			// of erroring like the other providers do. profs is non-nil only when
			// the profiles directory was read successfully, so a genuine read
			// error propagates rather than being mistaken for "no profiles".
			if len(args) == 0 && profs != nil && len(profs) == 0 {
				fmt.Fprintln(out, "azrl: no profile resolved — tenant-less sign-in into default ~/.azure")
				fmt.Fprintln(out, "      tip: run 'azrl login <name>' to save this as a profile")
				if err := runLogin("", g, loginPaste, out); err != nil {
					return err
				}
				acct, _ := azure.AccountShow()
				printSignedIn(out, acct)
				return nil
			}
			return rErr
		}
		if err := validAzureName(name); err != nil {
			return err
		}
		if newProfile {
			// First-login on a TTY with no saved profiles: create and sign in via
			// the shared tenant-less create path (runAzureInit).
			return runAzureInit(cmd, g, name, pwd, loginPaste, !loginNoLink)
		}
		conf, err := profile.LoadConf(name, config.ProfilesDir())
		if err != nil {
			// Explicit unknown name: confirm-then-create inline via the tenant-less
			// path (azure discovers the tenant on first login), matching gh/gcp/aws.
			if !confirmCreateProfile(cmd, "azrl", name, "tenant-less sign-in", loginYes) {
				return fmt.Errorf("azrl: no profile %q — pass --yes to create it or run interactively", name)
			}
			return runAzureInit(cmd, g, name, pwd, loginPaste, !loginNoLink)
		}
		if conf.BrowserCmd != "" {
			g.BrowserCmd = conf.BrowserCmd
		}
		cfgDir := filepath.Join(config.ProfilesDir(), name)
		os.MkdirAll(cfgDir, 0o755)
		os.Setenv("AZURE_CONFIG_DIR", cfgDir)
		fmt.Fprintf(out, "azrl: profile=%s tenant=%s\n", name, conf.Tenant)
		azure.CleanSlate(cfgDir, out)
		if err := runLogin(conf.Tenant, g, loginPaste, out); err != nil {
			return err
		}
		if conf.DefaultSub != "" {
			if err := azure.SetSubscription(conf.DefaultSub); err != nil {
				return fmt.Errorf("azrl: could not select subscription %q: %w", conf.DefaultSub, err)
			}
		}
		acct, _ := azure.AccountShow()
		expTenant := conf.TenantID
		if expTenant == "" {
			expTenant = conf.Tenant
		}
		if err := azure.AssertAccount(acct, expTenant, conf.ExpectUser); err != nil {
			return err
		}
		if err := profile.AzureScheme().Touch(name, config.ProfilesDir(), pwd); err != nil {
			return err
		}
		printSignedIn(out, acct)
		// The profile's session lives in its isolated dir; plain `az` in this
		// shell still uses ~/.azure. Offer to pin it so they match.
		offerEnvrc(pwd, out, os.Stdin)
		return nil
	},
}

func printSignedIn(out interface{ Write([]byte) (int, error) }, acct []byte) {
	var a profile.AccountJSON
	_ = json.Unmarshal(acct, &a)
	tenant := a.TenantDefaultDomain
	if tenant == "" {
		tenant = a.TenantID
	}
	fmt.Fprintf(out, "✓ azrl: signed in as %s (tenant %s, sub %s)\n", a.User.Name, tenant, a.Name)
}

func init() {
	loginCmd.Flags().BoolVar(&loginPaste, "paste", false, "Force the manual paste-line path (A)")
	loginCmd.Flags().BoolVarP(&loginYes, "yes", "y", false, "Create a missing profile without prompting.")
	loginCmd.Flags().BoolVar(&loginNoLink, "no-link", false, "Create without claiming this directory (skip the .azprofile pin).")
	RootCmd.AddCommand(loginCmd)
}
