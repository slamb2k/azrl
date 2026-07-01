package ui

import tea "github.com/charmbracelet/bubbletea"

// Run launches the azrl tabbed TUI (Azure | GitHub | …).
func Run() error {
	p := tea.NewProgram(NewTabs(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
