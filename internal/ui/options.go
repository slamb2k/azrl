package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

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

// view renders the overlay body (the container supplies banner + tab bar).
func (o optionsPicker) view() string {
	contentW := o.width - 4
	if contentW < 1 {
		contentW = 1
	}
	var b strings.Builder
	b.WriteString(paneTitle("PROVIDER TABS", true) + "\n\n")
	b.WriteString(mutedStyle.Render("Choose which providers appear as tabs (saved to azrl.conf).") + "\n\n")
	for i, p := range o.provs {
		box := mutedStyle.Render("[ ]")
		if o.checked[p.Name()] {
			box = successStyle.Render("[x]")
		}
		line := box + " " + providerIcon(p.Name()) + " " + p.Title()
		if i == o.cursor {
			line = box + " " + providerIcon(p.Name()) + " " + selBlockActive.Render(p.Title())
		}
		b.WriteString(truncateLine(line, contentW) + "\n\n")
	}
	b.WriteString(mutedStyle.Render("space toggle · ↵ save · esc cancel"))
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = padTo(truncateLine(l, contentW), contentW)
	}
	// Fill to the overlay height so the frame matches the tabs' frames.
	for len(lines) < o.height-2 {
		lines = append(lines, padTo("", contentW))
	}
	return frameStyle.Render(strings.Join(lines, "\n"))
}
