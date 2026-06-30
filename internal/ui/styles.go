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
	// selection marker; keycapStyle renders [l]-style accelerators.
	selectedStyle = lipgloss.NewStyle().Foreground(white).Bold(true)
	dotStyle      = lipgloss.NewStyle().Foreground(azureSky)
	keycapStyle   = lipgloss.NewStyle().Foreground(gray)

	// paneTitleStyle labels each column; dividerStyle draws the rules and the
	// vertical seam between the two panes.
	paneTitleStyle = lipgloss.NewStyle().Foreground(azureSky).Bold(true)
	dividerStyle   = lipgloss.NewStyle().Foreground(azureDeep)

	frameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(azureDeep).
			Padding(0, 1)
)
