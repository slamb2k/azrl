package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// radioOption is one selectable row in a radio group.
type radioOption struct {
	label string // human-facing action name
	key   string // single-rune hotkey accelerator (e.g. "l"); empty for none
	hint  string // short trailing note shown muted (optional)
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
func (r radio) view(width int) string {
	var lines []string
	for i, o := range r.options {
		marker := "○"
		mStyle := mutedStyle
		labelStyle := mutedStyle
		if i == r.cursor {
			marker = "◉"
			if r.focused {
				mStyle, labelStyle = accentStyle, selectedStyle
			} else {
				mStyle, labelStyle = dotStyle, lipgloss.NewStyle().Foreground(white)
			}
		}
		left := mStyle.Render(marker) + " " + labelStyle.Render(o.label)
		cap := ""
		if o.key != "" {
			cap = keycap(o.key)
		}
		// pad between label and keycap so caps align on the right edge.
		gap := width - lipgloss.Width(left) - lipgloss.Width(cap)
		if gap < 1 {
			gap = 1
		}
		lines = append(lines, left+strings.Repeat(" ", gap)+cap)
	}
	// One blank line between rows keeps each action visually distinct,
	// matching the profile pane's spacing.
	return strings.Join(lines, "\n\n")
}
