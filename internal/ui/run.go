package ui

import tea "github.com/charmbracelet/bubbletea"

// Run launches the azrl TUI.
func Run() error {
	p := tea.NewProgram(NewModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
