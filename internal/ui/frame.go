package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/profile"
)

// paneDims computes the shared content and two-pane column widths for a given
// terminal width. It is the single source of truth for the canonical layout,
// used by the provider tabs and the frame renderer so every
// tab lines its panes up identically. contentW is the room inside the frame
// (border + padding), leftW/rightW the two column widths flanking the seam.
func paneDims(width int) (contentW, leftW, rightW int) {
	contentW = width - 4
	if contentW < 1 {
		contentW = 1
	}
	leftW = contentW * 40 / 100
	if leftW < 18 {
		leftW = 18
	}
	rightW = contentW - leftW - 3
	if rightW < 10 {
		rightW = 10
	}
	return
}

// renderPaneFrame draws the canonical azrl layout that every tab shares so they
// look identical: a header rule, a centered identity strip, a rule, a two-pane
// body (left/right already rendered), a rule, a centered status line, and a
// centered footer — filled to the full terminal width and height and wrapped in
// the frame. All content lines are padded to the content width so the frame
// spans the terminal edge-to-edge, and truncated so no line ever overflows it.
func renderPaneFrame(width, height int, identity, left, right, leftFoot, status, footer string) string {
	contentW, leftW, _ := paneDims(width)
	center := func(s string) string { return lipgloss.PlaceHorizontal(contentW, lipgloss.Center, s) }

	// No rule above the header — the frame's top border already bounds it.
	head := lipgloss.JoinVertical(lipgloss.Left, center(identity), rule(contentW))
	foot := lipgloss.JoinVertical(lipgloss.Left, rule(contentW), center(status), center(footer))

	// Vertical fill: grow the body so the frame bottom sits near the terminal
	// bottom (frame border = 2 rows) instead of a short box with dead space below.
	bodyH := height - 2 - lipgloss.Height(head) - lipgloss.Height(foot)
	if bodyH < 1 {
		bodyH = 1
	}
	// leftFoot (the icon legend) anchors to the bottom of the left column,
	// padded away from the rows above; at tiny heights it follows them directly.
	if leftFoot != "" {
		ll := strings.Split(left, "\n")
		lf := strings.Split(leftFoot, "\n")
		for pad := bodyH - len(ll) - len(lf); pad > 0; pad-- {
			ll = append(ll, "")
		}
		left = strings.Join(append(ll, lf...), "\n")
	}
	body := joinColumns(left, right, leftW, contentW, bodyH)

	content := lipgloss.JoinVertical(lipgloss.Left, head, body, foot)
	// Normalize every line to exactly contentW: truncate overflow (invariant) and
	// pad short lines so the frame border reaches the terminal's right edge.
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = padTo(truncateLine(l, contentW), contentW)
	}
	return frameStyle.Render(strings.Join(lines, "\n"))
}

// renderProfilePane hand-renders a PROFILES(n) pane for a slice of profiles
// (selection bar on the focused row, muted details) without a bubbles list.
// Rows lead with their relevance-to-this-dir icon (scopeSlot: ● green cwd link,
// ● orange parent link, empty slot otherwise); the global default renders a
// trailing "⌁ default" tag. Renamed profiles render their label in the
// renamedStyle accent instead of a footnote legend. Segments are styled
// independently so the icon keeps its own colour on selected rows.
func renderProfilePane(profiles []profile.Listed, cursor int, mode selMode, touched bool, leftW int, scopes map[string]string) string {
	var b strings.Builder
	b.WriteString(paneTitle(fmt.Sprintf("PROFILES (%d)", len(profiles)), mode == selActive && touched))
	b.WriteString("\n\n")
	if len(profiles) == 0 {
		b.WriteString(mutedStyle.Render("  (none yet — ") + keycap("n") + mutedStyle.Render(" creates, ") + keycap("a") + mutedStyle.Render(" adopts)"))
		return b.String()
	}
	textW := leftW - 5 // selection bar/pad (2) + icon slot (3)
	if textW < 1 {
		textW = 1
	}
	// The most-active profile — the one that would be used right now — renders
	// bold: the dir-pinned row when one exists, else the global default.
	emph := ""
	for _, p := range profiles {
		switch scopes[p.Name] {
		case ScopeCwd, ScopeAncestor:
			emph = p.Name
		case scopeGlobal:
			if emph == "" {
				emph = p.Name
			}
		}
	}
	for i, p := range profiles {
		if i > 0 {
			// One blank line between rows keeps each two-line profile distinct,
			// matching the Azure list delegate's spacing.
			b.WriteString("\n")
		}
		selected := i == cursor
		nameStyle := lipgloss.NewStyle().Foreground(white)
		detailStyle := mutedStyle
		switch {
		case selected && mode == selActive:
			// The shared selection block: bright in the focused container.
			nameStyle = selBlockActive
			detailStyle = lipgloss.NewStyle().Foreground(azureSky)
		case selected && mode == selParent:
			// A child holds focus: this level's selection dims as the trail.
			nameStyle = selBlockParent
			detailStyle = lipgloss.NewStyle().Foreground(gray)
		}
		if p.Label != "" && p.Label != p.Name {
			// Renamed rows keep their italic through selection.
			nameStyle = nameStyle.Italic(true)
			if !selected {
				nameStyle = nameStyle.Foreground(whiteDim)
			}
		}
		if p.Name == emph {
			nameStyle = nameStyle.Bold(true)
		}
		line := scopeSlot(scopes[p.Name]) + nameStyle.Render(truncateLine(p.Display(), textW))
		if scopes[p.Name] == scopeGlobal {
			line += "  " + mutedStyle.Render("⌁ default")
		}
		b.WriteString(line + "\n")
		b.WriteString("   " + detailStyle.Render(truncateLine(p.Detail, textW)) + "\n")
	}
	return b.String()
}

// joinColumns zips two blocks into a two-pane body of exactly totalW columns,
// with a vertical seam between them; both columns are padded so the seam runs
// full height and the right edge lines up with the rules and frame. The body is
// grown to at least minH rows so it fills the available vertical space.
func joinColumns(left, right string, leftW, totalW, minH int) string {
	seam := dividerStyle.Render("│")
	rightW := totalW - leftW - 3
	if rightW < 0 {
		rightW = 0
	}
	ll := strings.Split(left, "\n")
	rl := strings.Split(right, "\n")
	n := len(ll)
	if len(rl) > n {
		n = len(rl)
	}
	if minH > n {
		n = minH
	}
	rows := make([]string, n)
	for i := 0; i < n; i++ {
		var l, r string
		if i < len(ll) {
			l = ll[i]
		}
		if i < len(rl) {
			r = rl[i]
		}
		rows[i] = padTo(l, leftW) + " " + seam + " " + padTo(r, rightW)
	}
	return strings.Join(rows, "\n")
}

// padTo right-pads s with spaces to a visible width of w (ANSI-aware).
func padTo(s string, w int) string {
	if gap := w - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// paneTitle renders a pane header: bold for the focused pane (the selection
// block below carries the strong cue), muted otherwise.
func paneTitle(s string, active bool) string {
	if active {
		return paneTitleStyle.Render(s)
	}
	return mutedStyle.Render(s)
}

func rule(w int) string {
	if w < 1 {
		w = 1
	}
	return dividerStyle.Render(strings.Repeat("─", w))
}
