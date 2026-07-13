package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/slamb2k/azrl/internal/browserpick"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
	"github.com/slamb2k/azrl/internal/ui"
	"github.com/spf13/cobra"
)

var whoamiJSON bool

// whoamiRow is one provider's effective identity for the current directory:
// which profile governs here, how it wins, and the browser command a login
// or console launched from this shell would actually use.
type whoamiRow struct {
	Provider      string `json:"provider"`
	Profile       string `json:"profile,omitempty"`
	Identity      string `json:"identity,omitempty"`
	Via           string `json:"via"` // shell | pointer | ancestor | gitconfig | ambient | none
	Dir           string `json:"dir,omitempty"`
	Pointer       string `json:"pointer,omitempty"`
	Browser       string `json:"browser,omitempty"`
	BrowserLabel  string `json:"browserLabel,omitempty"`
	BrowserSource string `json:"browserSource,omitempty"` // profile | env | global
}

// whoamiReport is the full `azrl whoami --json` shape.
type whoamiReport struct {
	Dir           string      `json:"dir"`
	ShellOverride string      `json:"shell_override,omitempty"`
	Providers     []whoamiRow `json:"providers"`
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show what is in effect in this directory: governing profile and browser per provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		rep := buildWhoami(provider.All(), cwd)
		if whoamiJSON {
			b, err := json.MarshalIndent(rep, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		}
		printWhoami(cmd.OutOrStdout(), rep)
		return nil
	},
}

// buildWhoami resolves, per provider, what governs cwd — shell override >
// cwd pointer > nearest ancestor pointer / git config > ambient default —
// and the effective browser (profile override > $AZRL_BROWSER_CMD > global).
// Disk + process-env only, best-effort, never nudges setup.
func buildWhoami(provs []provider.Provider, cwd string) whoamiReport {
	ov := ui.BuildOverview(provs, cwd)
	rep := whoamiReport{Dir: cwd, ShellOverride: os.Getenv("AZRL_PROFILE"), Providers: []whoamiRow{}}
	for _, p := range provs {
		row := whoamiRow{Provider: p.Name(), Via: "none"}
		if prov, name, ok := strings.Cut(rep.ShellOverride, ":"); ok && prov == p.Name() && name != "" {
			row.Profile, row.Via = name, "shell"
		} else if m := governingMapping(ov, p.Name()); m != nil {
			row.Dir, row.Pointer = m.Dir, m.Pointer
			switch {
			case m.Source == "gitconfig":
				row.Via = "gitconfig"
			case m.Scope == ui.ScopeAncestor:
				row.Via = "ancestor"
			default:
				row.Via = "pointer"
			}
			row.Profile = m.Profile
			if m.Profile == "" {
				row.Identity = m.Unmanaged
			}
		} else if a := ambientFor(ov, p.Name()); a != nil {
			row.Via, row.Profile, row.Identity = "ambient", a.Profile, a.Identity
		}
		if row.Profile != "" && row.Identity == "" {
			if st, err := p.Status(row.Profile, p.ProfilesDir()); err == nil {
				row.Identity = st.Identity
			}
		}
		row.Browser, row.BrowserLabel, row.BrowserSource = effectiveBrowser(p, row.Profile)
		rep.Providers = append(rep.Providers, row)
	}
	return rep
}

// governingMapping returns the one mapping row BuildOverview scoped to cwd
// (or its nearest ancestor) for the provider, nil when none governs.
func governingMapping(ov ui.Overview, prov string) *ui.MappingRow {
	for i, m := range ov.Mappings {
		if m.Provider == prov && (m.Scope == ui.ScopeCwd || m.Scope == ui.ScopeAncestor) {
			return &ov.Mappings[i]
		}
	}
	return nil
}

// ambientFor returns the provider's ambient row, nil when none.
func ambientFor(ov ui.Overview, prov string) *ui.AmbientRow {
	for i, a := range ov.Ambient {
		if a.Provider == prov {
			return &ov.Ambient[i]
		}
	}
	return nil
}

// effectiveBrowser resolves the browser command a bridged sign-in would use
// right now: the profile's own *_BROWSER_CMD, else the $AZRL_BROWSER_CMD
// process override, else the global BROWSER_CMD from azrl.conf.
func effectiveBrowser(p provider.Provider, name string) (cmd, label, source string) {
	if name != "" {
		cmdKey, labelKey := browserpick.Keys(p.Name())
		s, dir := p.Scheme(), p.ProfilesDir()
		if c := s.GetKey(name, dir, cmdKey); c != "" {
			return c, s.GetKey(name, dir, labelKey), "profile"
		}
	}
	if v := os.Getenv("AZRL_BROWSER_CMD"); v != "" {
		return v, "", "env"
	}
	if g, err := config.LoadGlobal(config.ProfilesDir()); err == nil && g.BrowserCmd != "" {
		return g.BrowserCmd, "", "global"
	}
	return "", "", ""
}

// printWhoami renders the per-provider table: non-TTY safe, no colour.
func printWhoami(w io.Writer, rep whoamiReport) {
	fmt.Fprintln(w, rep.Dir)
	for _, r := range rep.Providers {
		via := "—"
		switch r.Via {
		case "shell":
			via = "shell override"
		case "pointer":
			via = "via " + r.Pointer
		case "ancestor":
			via = "via ancestor " + r.Dir
		case "gitconfig":
			via = "via git config"
		case "ambient":
			via = "ambient"
		}
		browser := "—"
		if r.Browser != "" {
			b := r.Browser
			if r.BrowserLabel != "" {
				b = r.BrowserLabel
			}
			browser = fmt.Sprintf("%s (%s)", b, r.BrowserSource)
		}
		fmt.Fprintf(w, "  %-8s %-16s %-24s %-32s browser: %s\n",
			r.Provider, dash(r.Profile), dash(r.Identity), via, browser)
	}
}

func init() {
	whoamiCmd.Flags().BoolVar(&whoamiJSON, "json", false, "Output the per-provider effective view as JSON")
	RootCmd.AddCommand(whoamiCmd)
}
