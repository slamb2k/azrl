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
		fmt.Fprintf(out, "azrl: browser opened on %s (zero-paste path B)\n", g.BrowserHost)
	} else if g.IsLocal() {
		fmt.Fprintf(out, "azrl: browser opened locally (%s)\n", g.BrowserCmd)
	} else {
		fmt.Fprintf(out, "azrl: paste this on your LOCAL machine:\n\n%s\n\n", paste)
		if _, ok := bridge.VMHost(g); !ok {
			fmt.Fprintln(out, "azrl: (set VM_SSH_HOST in azrl.conf, or run `azrl setup`, to fill in your VM's SSH name)")
		}
	}
	fmt.Fprintln(out, "azrl: waiting for sign-in to complete...")
	if err := azure.WaitForLogin(lg, browsercapture.LoginTimeout()); err != nil {
		if g.IsLocal() {
			fmt.Fprintf(out, "✗ %v\n  Retry with: azrl az login\n", err)
		} else {
			vmHost, _ := bridge.VMHost(g)
			fmt.Fprintf(out, "✗ %v\n  Recover with:\n  %s\n", err,
				bridge.PasteLine(lg.Port, vmHost, g.BrowserCmd, lg.URL))
		}
		// The browser half of the flow never finished — only a human can.
		return withCode(ExitNeedsHuman, err)
	}
	return nil
}

// captureSession records the current az session as <name>.conf — creating it,
// or updating it in place while keeping the user-set label and browser
// mapping — and, when link is true, pins the directory via .azprofile. Touch
// runs regardless: with no pointer present it just records LAST_USED/LAST_DIR,
// not a mapping. It returns the conf written and whether it was new.
func captureSession(name, pwd string, link bool, out io.Writer) (profile.Conf, bool, error) {
	confPath := filepath.Join(config.ProfilesDir(), name+".conf")
	acctBytes, err := azure.AccountShow()
	if err != nil {
		return profile.Conf{}, false, needsHumanErr("azrl: not logged in for %q — run azrl az login first", name)
	}
	var acct profile.AccountJSON
	if err := json.Unmarshal(acctBytes, &acct); err != nil {
		return profile.Conf{}, false, err
	}
	var doms profile.DomainsJSON
	_ = json.Unmarshal(azure.Domains(), &doms)
	c := profile.BuildConf(acct, doms)
	created := true
	if existing, err := profile.LoadConf(name, config.ProfilesDir()); err == nil {
		created = false
		c.Label = existing.Label
		c.BrowserCmd = existing.BrowserCmd
		c.BrowserLabel = existing.BrowserLabel
	}
	if err := c.Write(confPath); err != nil {
		return c, created, err
	}
	if link {
		if err := os.WriteFile(filepath.Join(pwd, ".azprofile"), []byte(name+"\n"), 0o644); err != nil {
			return c, created, err
		}
	}
	if err := profile.AzureScheme().Touch(name, config.ProfilesDir(), pwd); err != nil {
		return c, created, err
	}
	verb := "wrote"
	if !created {
		verb = "updated"
	}
	if link {
		fmt.Fprintf(out, "azrl: %s %s and %s/.azprofile\n", verb, confPath, pwd)
	} else {
		fmt.Fprintf(out, "azrl: %s %s\n", verb, confPath)
	}
	return c, created, nil
}
