package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// optionsPicker is the settings overlay: a checklist of registered providers
// selecting which tabs the TUI shows. Space toggles, enter saves (at least
// one provider must stay checked), esc cancels. The choice persists to the
// global azrl.conf.
type optionsPicker struct {
	provs   []provider.Provider
	checked map[string]bool
	cursor  int
	width   int
	height  int
}

func newOptionsPicker(width, height int) optionsPicker {
	checked := map[string]bool{}
	for _, n := range config.EnabledProviders(config.ProfilesDir()) {
		checked[n] = true
	}
	return optionsPicker{
		provs:   preferredOrder(provider.All()),
		checked: checked,
		width:   width,
		height:  height,
	}
}

// update handles one key. saved carries the chosen provider names when enter
// commits ("" length zero otherwise); closed reports dismissal.
func (o optionsPicker) update(msg tea.KeyMsg) (optionsPicker, []string, bool) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return o, nil, true
	case "up", "k":
		if o.cursor > 0 {
			o.cursor--
		}
	case "down", "j":
		if o.cursor < len(o.provs)-1 {
			o.cursor++
		}
	case " ":
		name := o.provs[o.cursor].Name()
		o.checked[name] = !o.checked[name]
	case "enter":
		var names []string
		for _, p := range o.provs {
			if o.checked[p.Name()] {
				names = append(names, p.Name())
			}
		}
		if len(names) == 0 {
			// At least one provider must remain; ignore the commit.
			return o, nil, false
		}
		return o, names, true
	}
	return o, nil, false
}

// clickRow handles a mouse click on option row i: a different row just
// selects it (mirrors ↑/↓); clicking the already-selected row runs the
// overlay's enter (commits the checked set), matching click-again-confirms
// elsewhere in the TUI.
func (o optionsPicker) clickRow(i int) (optionsPicker, []string, bool) {
	if i < 0 || i >= len(o.provs) {
		return o, nil, false
	}
	if i != o.cursor {
		o.cursor = i
		return o, nil, false
	}
	return o.update(tea.KeyMsg{Type: tea.KeyEnter})
}

// view renders a compact bordered box; the container overlays it centered on
// the active tab so settings read as a popup, not a screen. The whole box is
// zone-marked "box:options" (outside-click dismiss) and each row "opt:<i>"
// (row click).
func (o optionsPicker) view() string {
	innerW := 40
	var b strings.Builder
	b.WriteString(paneTitle("PROVIDER TABS", true) + "\n\n")
	b.WriteString(mutedStyle.Render("Saved to azrl.conf") + "\n\n")
	for i, p := range o.provs {
		box := mutedStyle.Render("[ ]")
		if o.checked[p.Name()] {
			box = successStyle.Render("[x]")
		}
		title := p.Title()
		if i == o.cursor {
			title = selBlockActive.Render(title)
		}
		line := truncateLine(box+" "+ProviderIcon(p.Name())+" "+title, innerW)
		b.WriteString(zone.Mark(fmt.Sprintf("opt:%d", i), line) + "\n\n")
	}
	b.WriteString(lipgloss.PlaceHorizontal(innerW, lipgloss.Center, keyHelp("space", "toggle", "↵", "save", "esc", "cancel")))
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = padTo(truncateLine(l, innerW), innerW)
	}
	return zone.Mark("box:options", lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(azureBlue).
		Padding(0, 2).
		Render(strings.Join(lines, "\n")))
}
