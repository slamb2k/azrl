package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/slamb2k/azrl/internal/browserpick"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
	"github.com/slamb2k/azrl/internal/ui"
	"github.com/spf13/cobra"
)

var (
	whoamiJSON    bool
	whoamiExplain bool
	whoamiAll     bool
)

// whoamiRow is one provider's effective identity for the current directory:
// which profile governs here, how it wins, and the browser command a login
// or console launched from this shell would actually use.
type whoamiRow struct {
	Provider      string   `json:"provider"`
	Profile       string   `json:"profile,omitempty"`
	Identity      string   `json:"identity,omitempty"`
	Subscription  string   `json:"subscription,omitempty"` // azure only
	Via           string   `json:"via"`                    // shell | pointer | ancestor | gitconfig | ambient | none
	Dir           string   `json:"dir,omitempty"`
	Pointer       string   `json:"pointer,omitempty"`
	Browser       string   `json:"browser,omitempty"`
	BrowserLabel  string   `json:"browserLabel,omitempty"`
	BrowserSource string   `json:"browserSource,omitempty"` // profile | env | global
	Trace         []string `json:"trace,omitempty"`         // --explain only: the full resolution ladder
}

// whoamiReport is the full `azrl whoami --json` shape.
type whoamiReport struct {
	Dir           string      `json:"dir"`
	ShellOverride string      `json:"shell_override,omitempty"`
	Providers     []whoamiRow `json:"providers"`
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show what is in effect here (--all: every mapping, ambient default, and profile)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if whoamiAll {
			return runOverview(cmd, whoamiJSON)
		}
		cwd, _ := os.Getwd()
		rep := buildWhoami(provider.All(), cwd, whoamiExplain)
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
// With explain it also records every rung of that ladder as a trace line.
// Disk + process-env only, best-effort, never nudges setup.
func buildWhoami(provs []provider.Provider, cwd string, explain bool) whoamiReport {
	ov := ui.BuildOverview(provs, cwd)
	rep := whoamiReport{Dir: cwd, ShellOverride: os.Getenv("AZRL_PROFILE"), Providers: []whoamiRow{}}
	for _, p := range provs {
		row := whoamiRow{Provider: p.Name(), Via: "none"}
		shellProv, shellName, _ := strings.Cut(rep.ShellOverride, ":")
		shellWins := shellProv == p.Name() && shellName != ""
		m := governingMapping(ov, p.Name())
		a := ambientFor(ov, p.Name())
		switch {
		case shellWins:
			row.Profile, row.Via = shellName, "shell"
		case m != nil:
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
		case a != nil:
			row.Via, row.Profile, row.Identity = "ambient", a.Profile, a.Identity
		}
		if row.Profile != "" {
			if st, err := p.Status(row.Profile, p.ProfilesDir()); err == nil {
				if row.Identity == "" {
					row.Identity = st.Identity
				}
				row.Subscription = st.Subscription
			}
		}
		row.Browser, row.BrowserLabel, row.BrowserSource = effectiveBrowser(p, row.Profile)
		if explain {
			row.Trace = traceLadder(p, cwd, rep.ShellOverride, shellWins, m, a, row)
		}
		rep.Providers = append(rep.Providers, row)
	}
	return rep
}

// traceLadder renders one provider's full resolution ladder: every rung with
// what was found there and whether it won or sits shadowed under a higher one.
func traceLadder(p provider.Provider, cwd, override string, shellWins bool, m *ui.MappingRow, a *ui.AmbientRow, row whoamiRow) []string {
	verdict := func(wins, has bool) string {
		switch {
		case wins:
			return "  → in effect"
		case has:
			return "  (shadowed)"
		}
		return ""
	}
	var t []string

	switch {
	case override == "":
		t = append(t, "1. shell override    $AZRL_PROFILE not set")
	case shellWins:
		t = append(t, fmt.Sprintf("1. shell override    $AZRL_PROFILE=%s%s", override, verdict(true, true)))
	default:
		t = append(t, fmt.Sprintf("1. shell override    $AZRL_PROFILE=%s — different provider", override))
	}

	if m == nil {
		t = append(t, fmt.Sprintf("2. directory mapping no mapping governs %s (cwd or any parent)", tildePath(cwd)))
	} else {
		var what string
		switch {
		case m.Source == "gitconfig" && m.Profile == "":
			what = fmt.Sprintf("repo git config names %q (unmanaged — no profile has that identity)", m.Unmanaged)
		case m.Source == "gitconfig":
			what = fmt.Sprintf("repo git config in %s maps to %q", tildePath(m.Dir), m.Profile)
		case m.Scope == ui.ScopeAncestor:
			what = fmt.Sprintf("nearest ancestor %s names %q", tildePath(filepath.Join(m.Dir, m.Pointer)), m.Profile)
		default:
			what = fmt.Sprintf("%s in this directory names %q", m.Pointer, m.Profile)
		}
		if m.Conflict != nil {
			what += fmt.Sprintf(" — conflict: %s names %q, git config wins", m.Pointer, m.Conflict.PointerProfile)
		}
		t = append(t, "2. directory mapping "+what+verdict(!shellWins, true))
	}

	if a == nil {
		t = append(t, "3. ambient default   none on disk (no native "+p.Name()+" session)")
	} else {
		what := fmt.Sprintf("native default is %s (%s)", a.Identity, a.Source)
		if a.Profile != "" {
			what += fmt.Sprintf(" = profile %q", a.Profile)
		} else {
			what += " — unmanaged"
		}
		t = append(t, "3. ambient default   "+what+verdict(row.Via == "ambient", true))
	}

	cmdKey, _ := browserpick.Keys(p.Name())
	switch {
	case row.Profile == "":
		t = append(t, "4. browser           no governing profile — no per-profile override")
	case row.BrowserSource == "profile":
		t = append(t, fmt.Sprintf("4. browser           %s=%s on profile %q  → in effect", cmdKey, row.Browser, row.Profile))
	default:
		t = append(t, fmt.Sprintf("4. browser           %s unset on profile %q", cmdKey, row.Profile))
	}
	if v := os.Getenv("AZRL_BROWSER_CMD"); v != "" {
		t = append(t, "   ├ env             $AZRL_BROWSER_CMD="+v+verdict(row.BrowserSource == "env", true))
	} else {
		t = append(t, "   ├ env             $AZRL_BROWSER_CMD not set")
	}
	if v := globalBrowserConf(); v != "" {
		t = append(t, "   └ global          azrl.conf BROWSER_CMD="+v+verdict(row.BrowserSource == "global", true))
	} else {
		t = append(t, "   └ global          azrl.conf BROWSER_CMD not configured")
	}
	return t
}

// globalBrowserConf reads BROWSER_CMD straight off azrl.conf — unlike
// LoadGlobal it is never masked by the $AZRL_BROWSER_CMD process override,
// so the trace shows what the file actually says.
func globalBrowserConf() string {
	f, err := os.Open(filepath.Join(config.ProfilesDir(), "azrl.conf"))
	if err != nil {
		return ""
	}
	defer f.Close()
	m, err := config.ParseKV(f)
	if err != nil {
		return ""
	}
	if v := m["BROWSER_CMD"]; v != "" {
		return v
	}
	return m["LOCAL_BROWSER_CMD"]
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

// printWhoami renders the per-provider table. Columns size to their content
// and shrink to the terminal; colour and the ●/⌁ marks reuse the TUI's
// language (green = this dir, orange = parent, ⌁ = shell/ambient) and vanish
// on pipes and files.
func printWhoami(w io.Writer, rep whoamiReport) {
	fmt.Fprintln(w, cliHeading.Render("📁 "+tildePath(rep.Dir)))
	rows := make([][]string, 0, len(rep.Providers))
	for _, r := range rep.Providers {
		mark, via := " ", "—"
		switch r.Via {
		case "shell":
			mark, via = cliAccent.Render("⌁"), "shell override"
		case "pointer":
			mark, via = cliGood.Render("●"), "via "+r.Pointer
		case "ancestor":
			mark, via = cliParent.Render("●"), "via ancestor "+tildePath(r.Dir)
		case "gitconfig":
			mark, via = cliGood.Render("●"), "via git config"
		case "ambient":
			mark, via = cliDim.Render("⌁"), "ambient"
		}
		browser := cliDim.Render("—")
		if r.Browser != "" {
			b := r.Browser
			if r.BrowserLabel != "" {
				b = r.BrowserLabel
			}
			browser = b + " " + cliDim.Render("("+r.BrowserSource+")")
		}
		profile := dash(r.Profile)
		if r.Profile != "" {
			profile = cliBold.Render(r.Profile)
		}
		identity := cliValue.Render(dash(r.Identity))
		if r.Subscription != "" {
			identity += " " + cliDim.Render("("+r.Subscription+")")
		}
		rows = append(rows, []string{
			mark + " " + ui.ProviderIcon(r.Provider) + " " + r.Provider,
			profile, identity, cliDim.Render(via), cliDim.Render("browser:") + " " + browser,
		})
	}
	renderAligned(w, "  ", rows)
	for _, r := range rep.Providers {
		if len(r.Trace) == 0 {
			continue
		}
		fmt.Fprintln(w, "  "+ui.ProviderIcon(r.Provider)+" "+cliHeading.Render(r.Provider))
		for _, line := range r.Trace {
			if i := strings.Index(line, "   "); i > 0 {
				line = cliDim.Render(line[:i]) + line[i:]
			}
			line = strings.ReplaceAll(line, "→ in effect", cliGood.Render("→ in effect"))
			line = strings.ReplaceAll(line, "(shadowed)", cliDim.Render("(shadowed)"))
			fmt.Fprintf(w, "      %s\n", line)
		}
	}
}

func init() {
	whoamiCmd.Flags().BoolVar(&whoamiJSON, "json", false, "Output the per-provider effective view as JSON")
	whoamiCmd.Flags().BoolVar(&whoamiExplain, "explain", false, "Show the full resolution ladder per provider: every rung checked and why the winner won")
	whoamiCmd.Flags().BoolVar(&whoamiAll, "all", false, "Show the everywhere-view: MAPPINGS / AMBIENT / UNMAPPED PROFILES across all directories")
	RootCmd.AddCommand(whoamiCmd)
}
