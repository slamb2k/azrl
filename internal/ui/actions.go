package ui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
)

// opDoneMsg reports the result of a background action.
type opDoneMsg struct {
	msg string
	err error
}

// runUse links the current directory to name.
func runUse(name string) tea.Cmd {
	return func() tea.Msg {
		pwd, _ := os.Getwd()
		if err := profile.Use(name, config.ProfilesDir(), pwd); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{msg: fmt.Sprintf("linked this dir → %s", name)}
	}
}

// runDelete removes a profile.
func runDelete(name string) tea.Cmd {
	return func() tea.Msg {
		pwd, _ := os.Getwd()
		if _, err := profile.Remove(name, config.ProfilesDir(), pwd); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{msg: fmt.Sprintf("removed profile %s", name)}
	}
}
