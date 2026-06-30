package ui

import (
	"fmt"
	"os"
	"os/exec"

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

// runWriteEnvrc pins the shell to this dir's profile by writing an .envrc.
func runWriteEnvrc() tea.Cmd {
	return func() tea.Msg {
		pwd, _ := os.Getwd()
		wrote, err := profile.WriteEnvrc(pwd)
		if err != nil {
			return opDoneMsg{err: err}
		}
		if !wrote {
			return opDoneMsg{msg: ".envrc already present"}
		}
		return opDoneMsg{msg: "wrote .envrc — run direnv allow"}
	}
}

// handoffArgs maps an az-touching action to the azrl subcommand args it should
// run. login targets the selected profile; init/capture default to the current
// directory, so they take no positional argument.
func handoffArgs(key, profileName string) []string {
	switch key {
	case "l":
		if profileName == "" {
			return []string{"login"}
		}
		return []string{"login", profileName}
	case "i":
		return []string{"init"}
	case "c":
		return []string{"capture"}
	}
	return nil
}

// runHandoff suspends the TUI (releasing the terminal) and runs azrl <args> so
// the bridge/login flow streams its normal output, then resumes the TUI.
func runHandoff(args []string) tea.Cmd {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "azrl"
	}
	c := exec.Command(self, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return opDoneMsg{err: fmt.Errorf("%s exited: %w", args[0], err)}
		}
		return opDoneMsg{msg: fmt.Sprintf("%s complete", args[0])}
	})
}
