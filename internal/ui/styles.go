// Package ui implements azrl's Bubble Tea terminal interface.
package ui

import "github.com/charmbracelet/lipgloss"

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
	// selection marker; keycapStyle renders keystroke glyphs/accelerators.
	selectedStyle = lipgloss.NewStyle().Foreground(white).Bold(true)
	dotStyle      = lipgloss.NewStyle().Foreground(azureSky)
	keycapStyle   = lipgloss.NewStyle().Foreground(azureSky)

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

// keyGlyph renders a hotkey as a negative-squared Unicode capital (🅻-style,
// U+1F170+). U+FE0E pins text presentation so the glyph stays single-width and
// takes ANSI colour even for the letters terminals treat as emoji (A/B/O/P).
// Named keys render as short text in the same keycap style.
func keyGlyph(key string) string {
	switch key {
	case "delete":
		return "DEL"
	case "f5":
		return "F5"
	}
	if r := []rune(key); len(r) == 1 && r[0] >= 'a' && r[0] <= 'z' {
		return string(rune(0x1F170+r[0]-'a')) + "\uFE0E"
	}
	return key
}

// keycap renders a keystroke hint in the shared keycap colour.
func keycap(key string) string { return keycapStyle.Render(keyGlyph(key)) }
