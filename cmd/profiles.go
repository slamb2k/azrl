package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/slamb2k/azrl/internal/browserpick"
	"github.com/slamb2k/azrl/internal/provider"
	"github.com/slamb2k/azrl/internal/ui"
	"github.com/spf13/cobra"
)

var profilesJSON bool

// profilesMapping is one directory→profile association for `azrl profiles`.
type profilesMapping struct {
	Dir          string     `json:"dir"`
	Provider     string     `json:"provider"`
	Profile      string     `json:"profile"`
	Source       string     `json:"source"`
	Scope        string     `json:"scope"`
	Drifted      bool       `json:"drifted"`
	Expiry       *time.Time `json:"expiry"`
	Browser      string     `json:"browser,omitempty"`
	BrowserLabel string     `json:"browserLabel,omitempty"`
}

// profilesAmbient is one provider's native default identity for `azrl profiles`.
// Profile is null when the identity matches no saved profile (unmanaged).
type profilesAmbient struct {
	Provider string  `json:"provider"`
	Identity string  `json:"identity"`
	Source   string  `json:"source"`
	Profile  *string `json:"profile"`
}

// profilesRow is one unmapped profile's snapshot: the existing per-profile
// status shape, kept for the "unmapped" section.
type profilesRow struct {
	Provider     string     `json:"provider"`
	ProfileName  string     `json:"profileName"`
	Identity     string     `json:"identity"`
	Directory    string     `json:"directory"`
	Expiry       *time.Time `json:"expiry"`
	Drifted      bool       `json:"drifted"`
	LastUsed     time.Time  `json:"lastUsed"`
	Browser      string     `json:"browser,omitempty"`
	BrowserLabel string     `json:"browserLabel,omitempty"`
}

// assignedBrowser reads a profile's browser assignment (the *_BROWSER_CMD /
// *_BROWSER_LABEL keys `azrl browser` writes) straight off the conf file.
func assignedBrowser(p provider.Provider, name string) (cmd, label string) {
	if name == "" {
		return "", ""
	}
	cmdKey, labelKey := browserpick.Keys(p.Name())
	s, dir := p.Scheme(), p.ProfilesDir()
	return s.GetKey(name, dir, cmdKey), s.GetKey(name, dir, labelKey)
}

// profilesReport is the full `azrl profiles --json` shape: the same three sections
// the TUI landing view renders.
type profilesReport struct {
	ShellOverride string            `json:"shell_override,omitempty"`
	Mappings      []profilesMapping `json:"mappings"`
	Ambient       []profilesAmbient `json:"ambient"`
	Unmapped      []profilesRow     `json:"unmapped"`
}

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "Show mappings, ambient defaults, and unmapped profiles (who am I, everywhere)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		provs := provider.All()
		byName := map[string]provider.Provider{}
		for _, p := range provs {
			byName[p.Name()] = p
		}
		ov := ui.BuildOverview(provs, cwd)
		rep := profilesReport{Mappings: []profilesMapping{}, Ambient: []profilesAmbient{}, Unmapped: []profilesRow{}}
		for _, m := range ov.Mappings {
			bcmd, blabel := assignedBrowser(byName[m.Provider], m.Profile)
			rep.Mappings = append(rep.Mappings, profilesMapping{
				Dir: m.Dir, Provider: m.Provider, Profile: m.Profile,
				Source: m.Source, Scope: m.Scope, Drifted: m.Drifted, Expiry: m.Expiry,
				Browser: bcmd, BrowserLabel: blabel,
			})
		}
		for _, a := range ov.Ambient {
			row := profilesAmbient{Provider: a.Provider, Identity: a.Identity, Source: a.Source}
			if a.Profile != "" {
				p := a.Profile
				row.Profile = &p
			}
			rep.Ambient = append(rep.Ambient, row)
		}
		for _, u := range ov.Unmapped {
			st := u.Status
			bcmd, blabel := assignedBrowser(byName[u.Provider], st.ProfileName)
			rep.Unmapped = append(rep.Unmapped, profilesRow{
				Provider: u.Provider, ProfileName: st.ProfileName, Identity: st.Identity,
				Directory: st.Directory, Expiry: st.Expiry, Drifted: st.Drifted, LastUsed: st.LastUsed,
				Browser: bcmd, BrowserLabel: blabel,
			})
		}
		rep.ShellOverride = os.Getenv("AZRL_PROFILE")
		if profilesJSON {
			b, err := json.MarshalIndent(rep, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		}
		printProfilesSections(cmd.OutOrStdout(), ov, rep)
		return nil
	},
}

// printProfilesSections renders the three-section view. Columns size to
// their content and shrink to the terminal; colour reuses the TUI's language
// (green ● this dir, orange ↑ parent dir) and vanishes on pipes and files.
func printProfilesSections(w io.Writer, ov ui.Overview, rep profilesReport) {
	if rep.ShellOverride != "" {
		_, prof, ok := strings.Cut(rep.ShellOverride, ":")
		if !ok {
			prof = rep.ShellOverride
		}
		fmt.Fprintf(w, "%s shell override: %s — this terminal acts as %s\n\n",
			cliAccent.Render("⌁"), rep.ShellOverride, cliBold.Render(prof))
	}
	fmt.Fprintln(w, cliBold.Render("MAPPINGS"))
	if len(ov.Mappings) == 0 {
		fmt.Fprintln(w, cliDim.Render("  (none)"))
	}
	rows := [][]string{}
	for i, m := range ov.Mappings {
		mark := " "
		switch m.Scope {
		case ui.ScopeCwd:
			mark = cliGood.Render("●")
		case ui.ScopeAncestor:
			mark = cliParent.Render("↑")
		}
		src := m.Pointer
		if m.Source == "gitconfig" {
			src = "(git)"
		}
		target := m.Provider + ":" + cliBold.Render(m.Profile)
		var notes []string
		if m.Unmanaged != "" {
			target = m.Provider + ": " + m.Unmanaged
			notes = append(notes, cliDim.Render("unmanaged"))
		}
		if m.Conflict != nil {
			notes = append(notes, cliBad.Render(fmt.Sprintf("conflict: %s → %s (git config wins)", m.Pointer, m.Conflict.PointerProfile)))
		}
		if m.Drifted {
			notes = append(notes, cliParent.Render("⚠ drift"))
		}
		if ui.ExpiryActionable(m.Provider) && m.Expiry != nil && time.Until(*m.Expiry) <= 0 {
			notes = append(notes, cliBad.Render("⚠ expired"))
		}
		rows = append(rows, []string{mark + " " + tildePath(m.Dir), "→ " + target, cliDim.Render(src),
			cliDim.Render(browserDisp(rep.Mappings[i].Browser, rep.Mappings[i].BrowserLabel)), strings.Join(notes, " ")})
	}
	renderAligned(w, "  ", rows)
	fmt.Fprintln(w, cliBold.Render("AMBIENT"))
	if len(ov.Ambient) == 0 {
		fmt.Fprintln(w, cliDim.Render("  (none)"))
	}
	rows = rows[:0]
	for _, a := range ov.Ambient {
		target := cliDim.Render("unmanaged")
		if a.Profile != "" {
			target = "managed → " + cliBold.Render(a.Profile)
		}
		rows = append(rows, []string{ui.ProviderIcon(a.Provider) + " " + a.Provider, a.Identity, cliDim.Render(a.Source), target})
	}
	renderAligned(w, "  ", rows)
	fmt.Fprintln(w, cliBold.Render("UNMAPPED PROFILES"))
	if len(rep.Unmapped) == 0 {
		fmt.Fprintln(w, cliDim.Render("  (none)"))
	}
	rows = rows[:0]
	for _, r := range rep.Unmapped {
		exp := plainExpiry(r.Expiry)
		if exp == "expired" {
			exp = cliBad.Render("⚠ expired")
		}
		rows = append(rows, []string{r.Provider + ":" + cliBold.Render(r.ProfileName), cliDim.Render(dash(r.Identity)),
			cliDim.Render(browserDisp(r.Browser, r.BrowserLabel)), exp})
	}
	renderAligned(w, "  ", rows)
}

// browserDisp renders a profile's assigned browser for the plain table:
// the human label when the picker set one, else the raw command, else a dash.
func browserDisp(cmd, label string) string {
	if label != "" {
		return label
	}
	return dash(cmd)
}

// plainExpiry renders a relative expiry for the plain table, styling-free.
func plainExpiry(exp *time.Time) string {
	if exp == nil {
		return "—"
	}
	d := time.Until(*exp)
	switch {
	case d <= 0:
		return "expired"
	case d >= time.Hour:
		return fmt.Sprintf("in %dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("in %ds", int(d.Seconds()))
	}
}

// dash renders a blank field as an em dash for the plain table.
func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func init() {
	profilesCmd.Flags().BoolVar(&profilesJSON, "json", false, "Output the three-section snapshot as JSON")
	RootCmd.AddCommand(profilesCmd)
}
