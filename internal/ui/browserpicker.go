package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

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

// clickRow handles a mouse click on match row i: a different row selects it;
// the already-selected row runs enter (maps the profile and closes).
func (p browserPicker) clickRow(i int) (browserPicker, *browserpick.Profile, bool) {
	if i < 0 || i >= len(p.matches) {
		return p, nil, false
	}
	if i != p.cursor {
		p.cursor = i
		return p, nil, false
	}
	return p.update(tea.KeyMsg{Type: tea.KeyEnter})
}

// handleBrowserPickMouse routes a mouse event to the browser picker overlay
// while it's open: wheel moves its cursor; a left release inside a row
// selects it (click-again maps the profile, exactly like pressing ↵); a left
// release outside the box is esc (dismiss without mapping) — mirroring the
// options/dirpicker overlay semantics one level up in the tab container.
func (v providerTabView) handleBrowserPickMouse(msg tea.MouseMsg) (providerTabView, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		if v.browserPick.cursor < len(v.browserPick.matches)-1 {
			v.browserPick.cursor++
		}
		return v, nil
	case tea.MouseButtonWheelUp:
		if v.browserPick.cursor > 0 {
			v.browserPick.cursor--
		}
		return v, nil
	}
	if !leftRelease(msg) {
		return v, nil
	}
	if z := zone.Get("box:browser"); z == nil || !z.InBounds(msg) {
		np, _, closed := v.browserPick.update(tea.KeyMsg{Type: tea.KeyEscape})
		v.browserPick = &np
		if closed {
			v.browserPick = nil
			v.status = ""
			v.browserFor = ""
		}
		return v, nil
	}
	for i := range v.browserPick.matches {
		if z := zone.Get(fmt.Sprintf("bp:%d", i)); z != nil && z.InBounds(msg) {
			np, picked, closed := v.browserPick.clickRow(i)
			v.browserPick = &np
			if closed {
				v.browserPick = nil
				if picked != nil {
					v.applyBrowserMapping(picked.Command(), picked.Label())
				} else {
					v.status = ""
				}
				v.browserFor = ""
			}
			return v, nil
		}
	}
	return v, nil
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

// view renders the overlay body. The whole box is zone-marked "box:browser"
// (outside-click dismiss) and each rendered match row "bp:<i>" (row click).
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
		row := "  " + line
		if i == p.cursor {
			row = "  " + selBlockActive.Render(line)
		}
		b.WriteString(zone.Mark(fmt.Sprintf("bp:%d", i), row) + "\n")
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
	return zone.Mark("box:browser", lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(azureBlue).
		Padding(0, 2).
		Render(strings.Join(lines, "\n")))
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
