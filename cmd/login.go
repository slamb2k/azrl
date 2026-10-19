package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

// validAzureName guards Azure profile names with the shared rule every
// provider uses, refusing the reserved global-conf basename (azrl.conf).
var validAzureName = newValidName("azrl", "azrl")

func newAzureLoginCmd() *cobra.Command {
	var loginPaste, loginYes, loginNoLink bool
	var loginEnvrc *envrcFlags
	c := &cobra.Command{
		Use:   "login [profile]",
		Short: "Sign in via the remote-browser bridge (creates the profile on first use)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.ErrOrStderr()
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
					fmt.Fprintln(out, "      tip: run 'azrl az login <name>' to save this as a profile")
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
				return runAzureInit(cmd, g, name, pwd, loginPaste, !loginNoLink, loginEnvrc.mode())
			}
			conf, err := profile.LoadConf(name, config.ProfilesDir())
			if err != nil {
				// Explicit unknown name: confirm-then-create inline via the tenant-less
				// path (azure discovers the tenant on first login), matching gh/gcp/aws.
				if !confirmCreateProfile(cmd, "azrl", name, "tenant-less sign-in", loginYes) {
					return usageErr("azrl: no profile %q — pass --yes to create it or run interactively", name)
				}
				return runAzureInit(cmd, g, name, pwd, loginPaste, !loginNoLink, loginEnvrc.mode())
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
			offerEnvrc(pwd, loginEnvrc.mode(), out, cmd.InOrStdin())
			printApplyHint(cmd, "azrl", "azure", name)
			return nil
		},
	}
	c.Flags().BoolVar(&loginPaste, "paste", false, "Force the manual paste-line path (A)")
	c.Flags().BoolVarP(&loginYes, "yes", "y", false, "Create a missing profile without prompting.")
	c.Flags().BoolVar(&loginNoLink, "no-map", false, "Create without claiming this directory (skip the .azprofile pin).")
	loginEnvrc = addEnvrcFlags(c)
	return c
}

func printSignedIn(out interface{ Write([]byte) (int, error) }, acct []byte) {
	var a profile.AccountJSON
	_ = json.Unmarshal(acct, &a)
	tenant := a.TenantDefaultDomain
	if tenant == "" {
		tenant = a.TenantID
	}
	fmt.Fprintf(out, "%s azrl: signed in as %s (tenant %s, sub %s)\n", cliGood.Render("✓"), cliBold.Render(a.User.Name), tenant, a.Name)
}
