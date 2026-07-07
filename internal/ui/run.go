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
	// Run returns the final model on every exit path; centralize teardown here so
	// tab-owned resources (the dashboard's fsnotify watcher) are released whatever
	// the quit key or active tab. Best-effort — still surface the run error.
	final, err := p.Run()
	if tm, ok := final.(tabsModel); ok {
		_ = tm.Close()
	}
	return err
}
