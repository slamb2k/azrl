package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/slamb2k/azrl/internal/provider"
	"github.com/slamb2k/azrl/internal/ui"
	"github.com/spf13/cobra"
)

var statusJSON bool

// statusMapping is one directory→profile association for `azrl status`.
type statusMapping struct {
	Dir      string     `json:"dir"`
	Provider string     `json:"provider"`
	Profile  string     `json:"profile"`
	Source   string     `json:"source"`
	Scope    string     `json:"scope"`
	Drifted  bool       `json:"drifted"`
	Expiry   *time.Time `json:"expiry"`
}

// statusAmbient is one provider's native default identity for `azrl status`.
// Profile is null when the identity matches no saved profile (unmanaged).
type statusAmbient struct {
	Provider string  `json:"provider"`
	Identity string  `json:"identity"`
	Source   string  `json:"source"`
	Profile  *string `json:"profile"`
}

// statusRow is one unmapped profile's snapshot: the existing per-profile
// status shape, kept for the "unmapped" section.
type statusRow struct {
	Provider    string     `json:"provider"`
	ProfileName string     `json:"profileName"`
	Identity    string     `json:"identity"`
	Directory   string     `json:"directory"`
	Expiry      *time.Time `json:"expiry"`
	Drifted     bool       `json:"drifted"`
	LastUsed    time.Time  `json:"lastUsed"`
}

// statusReport is the full `azrl status --json` shape: the same three sections
// the TUI landing view renders.
type statusReport struct {
	Mappings []statusMapping `json:"mappings"`
	Ambient  []statusAmbient `json:"ambient"`
	Unmapped []statusRow     `json:"unmapped"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show mappings, ambient defaults, and unmapped profiles (who am I, everywhere)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		ov := ui.BuildOverview(provider.All(), cwd)
		rep := statusReport{Mappings: []statusMapping{}, Ambient: []statusAmbient{}, Unmapped: []statusRow{}}
		for _, m := range ov.Mappings {
			rep.Mappings = append(rep.Mappings, statusMapping{
				Dir: m.Dir, Provider: m.Provider, Profile: m.Profile,
				Source: m.Source, Scope: m.Scope, Drifted: m.Drifted, Expiry: m.Expiry,
			})
		}
		for _, a := range ov.Ambient {
			row := statusAmbient{Provider: a.Provider, Identity: a.Identity, Source: a.Source}
			if a.Profile != "" {
				p := a.Profile
				row.Profile = &p
			}
			rep.Ambient = append(rep.Ambient, row)
		}
		for _, u := range ov.Unmapped {
			st := u.Status
			rep.Unmapped = append(rep.Unmapped, statusRow{
				Provider: u.Provider, ProfileName: st.ProfileName, Identity: st.Identity,
				Directory: st.Directory, Expiry: st.Expiry, Drifted: st.Drifted, LastUsed: st.LastUsed,
			})
		}
		if statusJSON {
			b, err := json.MarshalIndent(rep, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		}
		printStatusSections(cmd.OutOrStdout(), ov, rep)
		return nil
	},
}

// printStatusSections renders the plain-text three-section view: non-TTY safe,
// no colour, no interactive elements.
func printStatusSections(w io.Writer, ov ui.Overview, rep statusReport) {
	fmt.Fprintln(w, "MAPPINGS")
	if len(ov.Mappings) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, m := range ov.Mappings {
		mark := " "
		switch m.Scope {
		case ui.ScopeCwd:
			mark = "●"
		case ui.ScopeAncestor:
			mark = "↑"
		}
		src := m.Pointer
		if m.Source == "gitconfig" {
			src = "(git)"
		}
		target := m.Provider + ":" + m.Profile
		note := ""
		if m.Unmanaged != "" {
			target = m.Provider + ": " + m.Unmanaged
			note = "  unmanaged"
		}
		if m.Conflict != nil {
			note += fmt.Sprintf("  conflict: %s → %s (git config wins)", m.Pointer, m.Conflict.PointerProfile)
		}
		if m.Drifted {
			note += "  drift"
		}
		if m.Expiry != nil && time.Until(*m.Expiry) <= 0 {
			note += "  expired"
		}
		fmt.Fprintf(w, "  %s %s → %s  %s%s\n", mark, m.Dir, target, src, note)
	}
	fmt.Fprintln(w, "AMBIENT")
	if len(ov.Ambient) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, a := range ov.Ambient {
		target := "unmanaged"
		if a.Profile != "" {
			target = "managed"
		}
		fmt.Fprintf(w, "  %s  %s  %s  %s\n", a.Provider, a.Identity, a.Source, target)
	}
	fmt.Fprintln(w, "UNMAPPED PROFILES")
	if len(rep.Unmapped) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, r := range rep.Unmapped {
		fmt.Fprintf(w, "  %s:%s · %s · %s\n", r.Provider, r.ProfileName, dash(r.Identity), plainExpiry(r.Expiry))
	}
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
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output the three-section snapshot as JSON")
	RootCmd.AddCommand(statusCmd)
}
