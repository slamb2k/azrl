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

	// paneTitleStyle labels each column; paneFocusStyle is the inverted chip on
	// the focused pane's title so the active pane is unmistakable; dividerStyle
	// draws the rules and the vertical seam between the two panes.
	paneTitleStyle = lipgloss.NewStyle().Foreground(azureSky).Bold(true)
	paneFocusStyle = lipgloss.NewStyle().Foreground(white).Background(azureBlue).Bold(true).Padding(0, 1)
	dividerStyle   = lipgloss.NewStyle().Foreground(azureDeep)

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

// scopeGlobal extends the overview's Scope values for profile rows: the
// profile is active as the provider's global default (ambient identity).
const scopeGlobal = "global"

// renamedStyle marks a relabeled profile's display name (bold gold italic),
// replacing the old "*" footnote legend.
var renamedStyle = lipgloss.NewStyle().Foreground(goldLight).Bold(true).Italic(true)

// scopeGlyph renders a profile row's trailing active-identity indicator: ●
// green when the pin is in the current dir, ● orange when inherited from a
// parent dir, 🌐 when the profile is the provider's global default, and ""
// when the identity is not active anywhere. When several scopes apply the
// caller passes the effective one (cwd > parent > global).
func scopeGlyph(scope string) string {
	switch scope {
	case ScopeCwd:
		return successStyle.Render("●")
	case ScopeAncestor:
		return lipgloss.NewStyle().Foreground(goldDeep).Render("●")
	case scopeGlobal:
		return "🌐"
	}
	return ""
}
