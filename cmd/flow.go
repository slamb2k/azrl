package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/bridge"
	"github.com/slamb2k/azrl/internal/browsercapture"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
)

// runLogin performs the capture→bridge→wait sequence. The caller is responsible
// for CleanSlate and for setting AZURE_CONFIG_DIR when isolation is wanted.
func runLogin(tenant string, g config.Global, forcePaste bool, out io.Writer) error {
	lg, err := azure.LoginCapture(tenant)
	if err != nil {
		if lg != nil {
			if lg.Capfile != "" {
				os.Remove(lg.Capfile)
			}
			if lg.Cmd != nil && lg.Cmd.Process != nil {
				_ = lg.Cmd.Process.Kill()
			}
		}
		return err
	}
	defer os.Remove(lg.Capfile)
	fmt.Fprintf(out, "azrl: callback port %s\n", lg.Port)
	tunnel, paste, err := bridge.Bridge(lg.Port, lg.URL, g, forcePaste)
	if err != nil {
		_ = lg.Cmd.Process.Kill()
		return err
	}
	if tunnel != nil {
		defer func() { _ = tunnel.Process.Kill() }()
		fmt.Fprintf(out, "azrl: browser opened on %s (zero-paste path B)\n", g.LocalHost)
	} else {
		fmt.Fprintf(out, "azrl: paste this on your LOCAL machine:\n\n%s\n\n", paste)
	}
	fmt.Fprintln(out, "azrl: waiting for sign-in to complete...")
	if err := azure.WaitForLogin(lg, browsercapture.LoginTimeout()); err != nil {
		fmt.Fprintf(out, "✗ %v\n  Recover with:\n  %s\n", err,
			bridge.PasteLine(lg.Port, g.VMHost, g.LocalBrowserCmd, lg.URL))
		return err
	}
	return nil
}

// captureSession records the current az session as <name>.conf + .azprofile.
func captureSession(name, pwd string, out io.Writer) error {
	confPath := filepath.Join(config.ProfilesDir(), name+".conf")
	if _, err := os.Stat(confPath); err == nil {
		return fmt.Errorf("azrl: %s already exists — remove it first", confPath)
	}
	acctBytes, err := azure.AccountShow()
	if err != nil {
		return fmt.Errorf("azrl: not logged in for %q — run azrl login first", name)
	}
	var acct profile.AccountJSON
	if err := json.Unmarshal(acctBytes, &acct); err != nil {
		return err
	}
	var doms profile.DomainsJSON
	_ = json.Unmarshal(azure.Domains(), &doms)
	c := profile.BuildConf(acct, doms)
	if err := c.Write(confPath); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pwd, ".azprofile"), []byte(name+"\n"), 0o644); err != nil {
		return err
	}
	if err := profile.AzureScheme().Touch(name, config.ProfilesDir(), pwd); err != nil {
		return err
	}
	fmt.Fprintf(out, "azrl: wrote %s and %s/.azprofile\n", confPath, pwd)
	return nil
}
