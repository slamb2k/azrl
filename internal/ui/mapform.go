package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// mapFormRow is one provider's slot in the map form: pick one profile — or
// none — to govern the current directory.
type mapFormRow struct {
	prov          provider.Provider
	profiles      []string
	sel           int    // 0 = none, i>0 = profiles[i-1]
	initial       int    // the cwd's own pointer at open time (0 = none)
	parentDir     string // non-empty: an ancestor's pointer governs this dir
	parentProfile string
}

// mapForm is the dashboard's map popup: a declarative editor of which
// profile (per provider) governs the current directory. Enter applies the
// diff against the open-time state — changed picks are mapped, cleared ones
// unmapped. (none) on an inherited mapping leaves the ancestor's pointer
// alone; picking a profile there shadows it with a cwd pointer.
type mapForm struct {
	rows   []mapFormRow
	cursor int
	dir    string
}

// newMapForm reads each provider's profile list and the cwd's current
// pointer state. Providers with no profiles are omitted; a form with zero
// rows means there is nothing to map yet.
func newMapForm(provs []provider.Provider, pwd string) mapForm {
	f := mapForm{dir: pwd}
	for _, p := range provs {
		listed, _ := p.ListProfiles(p.ProfilesDir())
		if len(listed) == 0 {
			continue
		}
		r := mapFormRow{prov: p}
		for _, l := range listed {
			r.profiles = append(r.profiles, l.Name)
		}
		if d, ok := p.Scheme().Locate(pwd); ok {
			b, err := os.ReadFile(filepath.Join(d, p.Scheme().Pointer))
			if name := strings.TrimSpace(string(b)); err == nil && name != "" {
				if d == pwd {
					for i, n := range r.profiles {
						if n == name {
							r.sel, r.initial = i+1, i+1
						}
					}
				} else {
					r.parentDir, r.parentProfile = d, name
				}
			}
		}
		f.rows = append(f.rows, r)
	}
	return f
}

// preselect points a provider's slot at the given profile — used when the
// form is opened from a dashboard row so that row's profile is the offer.
// A profile an ancestor already provides is left on (none): applying would
// only shadow the inherited pointer with a redundant cwd copy.
func (f *mapForm) preselect(providerName, profile string) {
	for ri := range f.rows {
		if f.rows[ri].prov.Name() != providerName || f.rows[ri].parentProfile == profile {
			continue
		}
		for i, n := range f.rows[ri].profiles {
			if n == profile {
				f.rows[ri].sel = i + 1
				f.cursor = ri
			}
		}
	}
}

// cycle advances the focused row's pick by delta, wrapping through (none).
func (f *mapForm) cycle(delta int) {
	if f.cursor < 0 || f.cursor >= len(f.rows) {
		return
	}
	r := &f.rows[f.cursor]
	n := len(r.profiles) + 1
	r.sel = ((r.sel+delta)%n + n) % n
}

// update handles one key. done reports the form closed; status carries the
// apply summary ("" on cancel).
func (f mapForm) update(msg tea.KeyMsg) (mapForm, bool, string) {
	switch msg.String() {
	case "esc", "ctrl+c", "q":
		return f, true, ""
	case "up", "k":
		if f.cursor > 0 {
			f.cursor--
		}
	case "down", "j":
		if f.cursor < len(f.rows)-1 {
			f.cursor++
		}
	case "right", "l", " ":
		f.cycle(1)
	case "left", "h":
		f.cycle(-1)
	case "enter":
		return f, true, f.apply()
	}
	return f, false, ""
}

// apply performs the diff: unmap cleared slots, map changed picks. The
// return is the dashboard status line.
func (f mapForm) apply() string {
	var parts []string
	failed := false
	for _, r := range f.rows {
		if r.sel == r.initial {
			continue
		}
		dir := r.prov.ProfilesDir()
		if r.sel == 0 {
			if _, err := r.prov.Scheme().Unlink(dir, f.dir); err != nil {
				parts, failed = append(parts, err.Error()), true
			} else {
				parts = append(parts, "unmapped "+r.prov.Name())
				if w := profile.EnvrcWarning(r.prov.Name(), f.dir); w != "" {
					parts = append(parts, w)
				}
			}
			continue
		}
		name := r.profiles[r.sel-1]
		if err := r.prov.Use(name, dir, f.dir); err != nil {
			parts, failed = append(parts, err.Error()), true
		} else {
			parts = append(parts, "mapped "+r.prov.Name()+":"+name)
		}
	}
	if len(parts) == 0 {
		return mutedStyle.Render("no changes")
	}
	if failed {
		return failureStyle.Render("✗ " + strings.Join(parts, " · "))
	}
	return successStyle.Render("✓ " + strings.Join(parts, " · "))
}

// choice renders the focused pick for a row: the profile name, or the
// (none) slot — which names the inherited profile when an ancestor governs.
func (r mapFormRow) choice() string {
	if r.sel > 0 {
		return r.profiles[r.sel-1]
	}
	if r.parentProfile != "" {
		return "(none — inherits " + r.parentProfile + ")"
	}
	return "(none)"
}

// view renders the centered popup box. The box is zone-marked
// "box:mapform" (outside-click dismiss) and each row "mapf:<i>".
func (f mapForm) view() string {
	innerW := 52
	var b strings.Builder
	b.WriteString(paneTitle("MAP THIS DIRECTORY", true) + "\n")
	b.WriteString(mutedStyle.Render("📁 "+displayDir(f.dir)) + "\n\n")
	for i, r := range f.rows {
		label := providerIcon(r.prov.Name()) + " " + r.prov.Title()
		pick := "◂ " + r.choice() + " ▸"
		if i == f.cursor {
			pick = selBlockActive.Render(pick)
		} else {
			pick = mutedStyle.Render(pick)
		}
		line := truncateLine(padTo(label, 18)+" "+pick, innerW)
		b.WriteString(zone.Mark(fmt.Sprintf("mapf:%d", i), line) + "\n")
		if r.parentDir != "" && r.sel == 0 {
			b.WriteString(mutedStyle.Render(truncateLine("   inherited from "+displayDir(r.parentDir), innerW)) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(lipgloss.PlaceHorizontal(innerW, lipgloss.Center,
		keyHelp("←→", "choose", "↑↓", "provider", "↵", "apply", "esc", "cancel")))
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = padTo(truncateLine(l, innerW), innerW)
	}
	return zone.Mark("box:mapform", lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(azureBlue).
		Padding(0, 2).
		Render(strings.Join(lines, "\n")))
}
