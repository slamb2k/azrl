// Package ui implements azrl's Bubble Tea terminal interface.
package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Azure-blue + gold palette. Blue carries structure and interaction; gold is
// the angel/halo signature, spent only on the active control so the eye lands
// on "what will happen."
var (
	azureBlue = lipgloss.Color("#2599f7")
	azureSky  = lipgloss.Color("#7cc4ff")
	azureDeep = lipgloss.Color("#0a4d8c")
	gold      = lipgloss.Color("#f2c14e")
	goldLight = lipgloss.Color("#ffe6a3")
	goldDeep  = lipgloss.Color("#d99a2b")
	white     = lipgloss.Color("#f5f7fa")
	whiteDim  = lipgloss.Color("#b9c0c8")
	grayDeep  = lipgloss.Color("#565c64")
	green     = lipgloss.Color("#3fb950")
	red       = lipgloss.Color("#f85149")
	gray      = lipgloss.Color("#8b949e")
)

var (
	accentStyle  = lipgloss.NewStyle().Foreground(gold).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
	failureStyle = lipgloss.NewStyle().Foreground(red).Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(gray)

	// selectedStyle marks the focused radio row; dotStyle is the inactive-pane
	// selection marker; keycapChipStyle renders accelerator hints as
	// reverse-video keycap chips.
	selectedStyle   = lipgloss.NewStyle().Foreground(white).Bold(true)
	dotStyle        = lipgloss.NewStyle().Foreground(azureSky)
	keycapChipStyle = lipgloss.NewStyle().Foreground(white).Background(azureDeep).Bold(true)

	// paneTitleStyle labels each column (bold when its pane is focused);
	// dividerStyle draws the rules and the vertical seam between the panes.
	paneTitleStyle = lipgloss.NewStyle().Foreground(azureSky).Bold(true)
	dividerStyle   = lipgloss.NewStyle().Foreground(azureDeep)

	// One selection language everywhere: the block in the focused container is
	// bright blue; each ANCESTOR retains its selection as a darker block (the
	// trail points up: tab bar → profiles → detail); descendants show no
	// selection until entered.
	selBlockActive = lipgloss.NewStyle().Foreground(white).Background(azureBlue).Bold(true)
	selBlockParent = lipgloss.NewStyle().Foreground(whiteDim).Background(azureDeep)

	frameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(azureDeep).
			Padding(0, 1)
)

// keyGlyph renders a hotkey's label: uppercase for letters (keycaps show
// capitals), short names for special keys. Plain ASCII on purpose — Unicode
// letter-forms (negative-squared, circled) are unmapped in many monospace
// fonts and shrink to unreadable substitutes at cell size.
func keyGlyph(key string) string {
	switch key {
	case "delete":
		return "DEL"
	case "f5":
		return "F5"
	}
	return strings.ToUpper(key)
}

// keycap renders a keystroke hint as a reverse-video chip (` L `), the same
// visual language as the focused-pane title.
func keycap(key string) string { return keycapChipStyle.Render(" " + keyGlyph(key) + " ") }

// renamedStyle marks a relabeled profile's display name (dull-white italic),
// replacing the old "*" footnote legend.
var renamedStyle = lipgloss.NewStyle().Foreground(whiteDim).Italic(true)

// scopeGlobal and scopeElsewhere extend the overview's Scope values for
// profile rows: the provider's global default (ambient identity match), and a
// profile mapped to some directory that doesn't govern this one.
const (
	scopeGlobal    = "global"
	scopeElsewhere = "elsewhere"
)

// scopeSlot renders a profile row's leading icon as a fixed-width slot so
// names align regardless of glyph (🌐 is double-width). Relevance grades the
// colour: ● green when a pin in the current dir makes the profile effective,
// ● orange when the pin is inherited from a parent dir, 🌐 only for the
// provider's global default, ● dark-white for identities mapped elsewhere
// (irrelevant here), and ● deep-grey for profiles mapped nowhere at all.
func scopeSlot(scope string) string {
	switch scope {
	case ScopeCwd:
		return successStyle.Render("●") + "  "
	case ScopeAncestor:
		return lipgloss.NewStyle().Foreground(goldDeep).Render("●") + "  "
	case scopeGlobal:
		return "🌐 "
	case scopeElsewhere:
		return lipgloss.NewStyle().Foreground(whiteDim).Render("●") + "  "
	}
	return lipgloss.NewStyle().Foreground(grayDeep).Render("●") + "  "
}

// keyHelp renders alternating key/label pairs as keycap chips with muted
// labels, dot-separated — the one way every footer spells its bindings.
func keyHelp(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, keycap(pairs[i])+" "+mutedStyle.Render(pairs[i+1]))
	}
	return strings.Join(parts, mutedStyle.Render(" · "))
}

// keyHelpFit renders core then optional key/label pairs, dropping optional
// items from the right (least important last) until the bar fits width —
// narrow terminals keep the essentials instead of truncating mid-chip.
func keyHelpFit(width int, core, optional []string) string {
	pairs := append(append([]string{}, core...), optional...)
	for len(pairs) > len(core) {
		if s := keyHelp(pairs...); width <= 0 || lipgloss.Width(s) <= width {
			return s
		}
		pairs = pairs[:len(pairs)-2]
	}
	return keyHelp(core...)
}
