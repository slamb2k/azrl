package ui

import tea "github.com/charmbracelet/bubbletea"

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
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
