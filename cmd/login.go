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

var loginPaste bool

var loginCmd = &cobra.Command{
	Use:   "login [profile]",
	Short: "Sign in via the remote-browser bridge",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		g, err := config.LoadGlobal(config.ProfilesDir())
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		pwd, _ := os.Getwd()
		prov := azure.NewProvider()
		name, profs, rErr := resolveLoginTargetWithProfiles(cmd, prov, args, "azrl")
		if rErr != nil {
			// Preserve azure's tenant-less fallback: with no arg, no directory
			// pin and no saved profiles, sign in to the default ~/.azure instead
			// of erroring like the other providers do. profs is non-nil only when
			// the profiles directory was read successfully, so a genuine read
			// error propagates rather than being mistaken for "no profiles".
			if len(args) == 0 && profs != nil && len(profs) == 0 {
				fmt.Fprintln(out, "azrl: no profile resolved — tenant-less sign-in into default ~/.azure")
				fmt.Fprintln(out, "      tip: run 'azrl init <name>' to save this as a profile")
				if err := runLogin("", g, loginPaste, out); err != nil {
					return err
				}
				acct, _ := azure.AccountShow()
				printSignedIn(out, acct)
				return nil
			}
			return rErr
		}
		conf, err := profile.LoadConf(name, config.ProfilesDir())
		if err != nil {
			return err
		}
		cfgDir := filepath.Join(config.ProfilesDir(), name)
		os.MkdirAll(cfgDir, 0o755)
		os.Setenv("AZURE_CONFIG_DIR", cfgDir)
		fmt.Fprintf(out, "azrl: profile=%s tenant=%s\n", name, conf.Tenant)
		azure.CleanSlate(cfgDir)
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
	RootCmd.AddCommand(loginCmd)
}
