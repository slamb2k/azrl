package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/spf13/cobra"
)

// runAzureInit performs the tenant-less sign-in and records it as the named
// profile: isolate an AZURE_CONFIG_DIR, CleanSlate, sign in with no --tenant,
// capture the session as <name>.conf (plus .azprofile when link is true), then
// offer an .envrc. It is the shared create-and-sign-in path for `azrl login` —
// used both by the first-login (newProfile) prompt and by an explicit unknown
// profile name.
func runAzureInit(cmd *cobra.Command, g config.Global, name, pwd string, forcePaste, link bool) error {
	confPath := filepath.Join(config.ProfilesDir(), name+".conf")
	if _, err := os.Stat(confPath); err == nil {
		return fmt.Errorf("azrl: %s already exists — remove it first", confPath)
	}
	cfgDir := filepath.Join(config.ProfilesDir(), name)
	os.MkdirAll(cfgDir, 0o755)
	os.Setenv("AZURE_CONFIG_DIR", cfgDir)
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "azrl: init profile=%s (tenant-less sign-in)\n", name)
	azure.CleanSlate(cfgDir, out)
	if err := runLogin("", g, forcePaste, out); err != nil {
		return err
	}
	if err := captureSession(name, pwd, link, out); err != nil {
		return err
	}
	offerEnvrc(pwd, out, os.Stdin)
	return nil
}
