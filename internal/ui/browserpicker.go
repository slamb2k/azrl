package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/browserpick"
	"github.com/slamb2k/azrl/internal/config"
)

// browserProfilesMsg carries the async discovery result for one azrl profile.
type browserProfilesMsg struct {
	forProfile string
	profiles   []browserpick.Profile
	err        error
}

// browserPicker is the fuzzy overlay listing discovered browser profiles.
type browserPicker struct {
	input   textinput.Model
	all     []browserpick.Profile
	matches []browserpick.Profile
	cursor  int
}

func newBrowserPicker(profiles []browserpick.Profile, ident string) browserPicker {
	if ident != "" {
		sort.SliceStable(profiles, func(i, j int) bool {
			return profiles[i].Email == ident && profiles[j].Email != ident
		})
	}
	ti := textinput.New()
	ti.Placeholder = "filter"
	ti.Focus()
	p := browserPicker{input: ti, all: profiles}
	p.refilter()
	return p
}

func (p *browserPicker) refilter() {
	pattern := p.input.Value()
	type scored struct {
		bp    browserpick.Profile
		score int
	}
	var hits []scored
	for _, b := range p.all {
		if sc := fuzzyScore(pattern, b.Label()+" "+b.Email); sc >= 0 {
			hits = append(hits, scored{b, sc})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	p.matches = p.matches[:0]
	for _, h := range hits {
		p.matches = append(p.matches, h.bp)
	}
	if p.cursor >= len(p.matches) {
		p.cursor = 0
	}
}

// update returns (picker, picked, closed): picked is non-nil only on enter.
func (p browserPicker) update(msg tea.KeyMsg) (browserPicker, *browserpick.Profile, bool) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return p, nil, true
	case "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return p, nil, false
	case "down":
		if p.cursor < len(p.matches)-1 {
			p.cursor++
		}
		return p, nil, false
	case "enter":
		if p.cursor < len(p.matches) {
			bp := p.matches[p.cursor]
			return p, &bp, true
		}
		return p, nil, false
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	_ = cmd
	p.refilter()
	return p, nil, false
}

func (p browserPicker) view() string {
	innerW := 48
	var b strings.Builder
	b.WriteString(paneTitle("BROWSER PROFILE", true) + "\n\n")
	b.WriteString(p.input.View() + "\n\n")
	for i, m := range p.matches {
		email := m.Email
		if email == "" {
			email = "(not signed in)"
		}
		line := truncateLine(m.Label()+"  "+mutedStyle.Render(email), innerW-4)
		if i == p.cursor {
			b.WriteString("  " + selBlockActive.Render(line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	if len(p.matches) == 0 {
		b.WriteString(mutedStyle.Render("  (no matches)") + "\n")
	}
	b.WriteString("\n" + lipgloss.PlaceHorizontal(innerW, lipgloss.Center,
		keyHelp("↑↓", "select", "↵", "map", "esc", "cancel")))
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = padTo(truncateLine(l, innerW), innerW)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(azureBlue).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))
}

// browserAction starts async discovery for the selected profile.
func browserAction(v *providerTabView) tea.Cmd {
	name := v.selected()
	if name == "" {
		v.status = mutedStyle.Render("no profile selected")
		return nil
	}
	v.browserFor = name
	v.status = mutedStyle.Render("looking for browser profiles on the local machine…")
	return discoverBrowsersCmd(name)
}

// discoverBrowsersCmd loads the global config and discovers the local
// machine's browser profiles for name, reporting the result as a
// browserProfilesMsg. Shared by the provider tabs and the Azure home model.
func discoverBrowsersCmd(name string) tea.Cmd {
	return func() tea.Msg {
		g, err := config.LoadGlobal(config.ProfilesDir())
		if err != nil {
			return browserProfilesMsg{forProfile: name, err: err}
		}
		ps, derr := browserpick.Discover(g)
		return browserProfilesMsg{forProfile: name, profiles: ps, err: derr}
	}
}
