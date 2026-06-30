// Package ui implements azrl's Bubble Tea terminal interface.
package ui

import "github.com/charmbracelet/lipgloss"

// Azure-blue + gold palette.
var (
	azureBlue = lipgloss.Color("#2599f7")
	azureDeep = lipgloss.Color("#0a4d8c")
	gold      = lipgloss.Color("#f2c14e")
	white     = lipgloss.Color("#f5f7fa")
	green     = lipgloss.Color("#3fb950")
	red       = lipgloss.Color("#f85149")
	gray      = lipgloss.Color("#8b949e")
)

var (
	titleStyle   = lipgloss.NewStyle().Foreground(azureBlue).Bold(true)
	accentStyle  = lipgloss.NewStyle().Foreground(gold).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
	failureStyle = lipgloss.NewStyle().Foreground(red).Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(gray)
	panelStyle   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(azureDeep).
			Padding(0, 1)
)
