package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/envdetect"
	"github.com/slamb2k/azrl/internal/ui"
	"github.com/spf13/cobra"
)

var setupYes, setupPrint bool

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Detect your environment and write azrl.conf",
	Long: "Detect whether azrl runs locally (WSL/macOS/desktop) or on a remote SSH\n" +
		"VM and write ~/.azure-profiles/azrl.conf accordingly. Interactive by\n" +
		"default; --yes writes the recommended config non-interactively and --print\n" +
		"shows the resolved config without writing.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		cands := envdetect.Detect(envdetect.RealEnv())

		if setupPrint {
			g := recommendedGlobal(cands, false)
			printResolved(out, g)
			return nil
		}
		if setupYes {
			g := recommendedGlobal(cands, true)
			return writeConf(config.ProfilesDir(), g, out)
		}
		g, ok, err := ui.RunSetupWizard(cands)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "azrl: setup cancelled — no changes written")
			return nil
		}
		return writeConf(config.ProfilesDir(), g, out)
	},
}

// recommendedGlobal folds the recommended candidate into a Global. When
// fillDefault is set (the non-interactive --yes path), an empty BrowserCmd — a
// remote candidate that would normally ask the dev-machine OS — falls back to
// xdg-open so the written config is valid; the user re-runs `azrl setup` to change.
func recommendedGlobal(cands []envdetect.Candidate, fillDefault bool) config.Global {
	c := envdetect.Candidate{}
	if len(cands) > 0 {
		c = cands[0]
		for _, cand := range cands {
			if cand.Recommended {
				c = cand
				break
			}
		}
	}
	g := config.Global{BrowserCmd: c.BrowserCmd, BrowserHost: c.BrowserHost, VMSSHHost: c.VMSSHHost}
	if fillDefault && g.BrowserCmd == "" {
		g.BrowserCmd = "xdg-open"
	}
	return g
}

// writeConf backs up any existing azrl.conf to azrl.conf.bak, then writes g.
func writeConf(dir string, g config.Global, out io.Writer) error {
	path := filepath.Join(dir, "azrl.conf")
	if b, err := os.ReadFile(path); err == nil {
		if werr := os.WriteFile(path+".bak", b, 0o644); werr != nil {
			return werr
		}
		fmt.Fprintf(out, "azrl: backed up existing config to %s.bak\n", path)
	}
	if err := g.Write(path); err != nil {
		return err
	}
	mode := "remote"
	if g.IsLocal() {
		mode = "local"
	}
	fmt.Fprintf(out, "azrl: wrote %s (%s mode, BROWSER_CMD=%s)\n", path, mode, g.BrowserCmd)
	fmt.Fprintln(out, "azrl: next — azrl login <name> creates a profile and signs in (bare azrl opens the dashboard)")
	return nil
}

// printResolved prints the resolved config keys without writing anything.
func printResolved(out io.Writer, g config.Global) {
	fmt.Fprintf(out, "BROWSER_CMD=%s\n", g.BrowserCmd)
	fmt.Fprintf(out, "BROWSER_HOST=%s\n", g.BrowserHost)
	fmt.Fprintf(out, "VM_SSH_HOST=%s\n", g.VMSSHHost)
}

// loadGlobalOrSetup loads the global config, nudging the user through setup when
// it is missing, placeholder, or invalid. On a TTY it launches the wizard and
// reloads; otherwise it returns the underlying problem with a "run `azrl setup`"
// hint. It is the entry point for commands that need global config.
func loadGlobalOrSetup(out io.Writer) (config.Global, error) {
	dir := config.ProfilesDir()
	g, err := config.LoadGlobal(dir)
	if err == nil && !config.IsPlaceholder(g) {
		return g, nil
	}
	if !isInteractive() {
		if err != nil {
			return g, fmt.Errorf("%w\n  run `azrl setup` to configure azrl.conf", err)
		}
		return g, fmt.Errorf("azrl: azrl.conf still has placeholder values — run `azrl setup` to configure it")
	}
	fmt.Fprintln(out, "azrl: azrl.conf needs configuring — launching setup…")
	cands := envdetect.Detect(envdetect.RealEnv())
	ng, ok, werr := ui.RunSetupWizard(cands)
	if werr != nil {
		return g, werr
	}
	if !ok {
		return g, fmt.Errorf("azrl: setup cancelled")
	}
	if werr := writeConf(dir, ng, out); werr != nil {
		return g, werr
	}
	return config.LoadGlobal(dir)
}

func init() {
	setupCmd.Flags().BoolVarP(&setupYes, "yes", "y", false, "Non-interactive: write the recommended config without prompting.")
	setupCmd.Flags().BoolVar(&setupPrint, "print", false, "Print the resolved config and write nothing.")
	RootCmd.AddCommand(setupCmd)
}
