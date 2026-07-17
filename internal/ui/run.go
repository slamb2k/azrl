package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// Run launches the azrl tabbed TUI on the cross-provider dashboard (the default
// landing view).
func Run() error {
	return runTabs(NewTabs())
}

// RunGitHub launches the tabbed TUI preselected on the GitHub tab (ghrl alias).
func RunGitHub() error {
	return runTabs(NewTabsForProvider("github"))
}

func runTabs(m tabsModel) error {
	zone.NewGlobal()
	defer zone.Close()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
