package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// radioOption is one selectable row in a radio group.
type radioOption struct {
	label    string // human-facing action name
	key      string // single-rune hotkey accelerator (e.g. "l"); empty for none
	hint     string // short trailing note shown muted (optional)
	disabled bool   // rendered dim; the hint carries the reason it can't apply
}

// radio is a vertical single-select control rendered with ◉/○ markers.
type radio struct {
	options []radioOption
	cursor  int
	focused bool
}

func newRadio(opts []radioOption) radio { return radio{options: opts} }

func (r *radio) up() {
	if r.cursor > 0 {
		r.cursor--
	}
}

func (r *radio) down() {
	if r.cursor < len(r.options)-1 {
		r.cursor++
	}
}

// selected returns the option under the cursor.
func (r radio) selected() radioOption { return r.options[r.cursor] }

// selectByKey moves the cursor to the option whose hotkey equals k and reports
// whether a match was found.
func (r *radio) selectByKey(k string) bool {
	for i, o := range r.options {
		if o.key != "" && o.key == k {
			r.cursor = i
			return true
		}
	}
	return false
}

// view renders the group. width is the column width used to right-align keycaps.
// view renders the group: keycap chips on the left of each label and a short
// muted hint to the right when it fits. The selected row carries the shared
// bright selection block only while this pane holds focus — it is the deepest
// level, so it never renders a parent shade.
func (r radio) view(width int) string {
	capW := 0
	for _, o := range r.options {
		if o.key != "" {
			if w := lipgloss.Width(keycap(o.key)); w > capW {
				capW = w
			}
		}
	}
	var lines []string
	for i, o := range r.options {
		cap := strings.Repeat(" ", capW)
		if o.key != "" {
			c := keycap(o.key)
			cap = c + strings.Repeat(" ", capW-lipgloss.Width(c))
		}
		labelStyle := lipgloss.NewStyle().Foreground(white)
		if o.disabled {
			labelStyle = mutedStyle
		}
		if i == r.cursor && r.focused {
			// Selection shows only while this (deepest) level holds focus; a
			// disabled row under the cursor takes the dim parent shade so it
			// reads as "selected, but not runnable".
			labelStyle = selBlockActive
			if o.disabled {
				labelStyle = selBlockParent
			}
		}
		sep := " "
		if capW > 0 {
			sep = "  "
		}
		line := cap + sep + labelStyle.Render(o.label)
		if o.hint != "" {
			if room := width - lipgloss.Width(line) - 2; room >= lipgloss.Width(o.hint) {
				line += "  " + mutedStyle.Render(o.hint)
			}
		}
		lines = append(lines, truncateLine(line, width))
	}
	// One blank line between rows keeps each action visually distinct,
	// matching the profile pane's spacing.
	return strings.Join(lines, "\n\n")
}
