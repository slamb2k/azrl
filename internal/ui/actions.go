package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/slamb2k/azrl/internal/profile"
)

// opDoneMsg reports the result of a background action.
type opDoneMsg struct {
	msg string
	err error
}

// runWriteEnvrc links the shell to this dir's profile by writing an .envrc.
func runWriteEnvrc() tea.Cmd {
	return func() tea.Msg {
		pwd, _ := os.Getwd()
		dir := pwd
		if d, ok := profile.LocateAzprofile(pwd); ok {
			dir = d
		}
		wrote, err := profile.WriteEnvrc(dir)
		if err != nil {
			return opDoneMsg{err: err}
		}
		if !wrote {
			return opDoneMsg{msg: ".envrc already present"}
		}
		if ran, aerr := profile.DirenvAllow(dir); ran && aerr == nil {
			return opDoneMsg{msg: "wrote .envrc + direnv allow — shell now follows this profile"}
		}
		return opDoneMsg{msg: "wrote .envrc — run direnv allow to activate"}
	}
}

// groupArgs builds a provider-group invocation, accounting for the promoted
// ghrl binary where the gh group's verbs sit at the top level.
func groupArgs(group string, rest ...string) []string {
	if group == "" {
		return rest
	}
	if group == "gh" {
		if self, err := os.Executable(); err == nil &&
			strings.TrimSuffix(filepath.Base(self), ".exe") == "ghrl" {
			return rest
		}
	}
	return append([]string{group}, rest...)
}

// runHandoff suspends the TUI (releasing the terminal) and runs azrl <args> so
// the bridge/login flow streams its normal output, then resumes the TUI.
// Bubble Tea's RestoreTerminal (v1.3.x) re-enables the alt screen, bracketed
// paste, and focus reporting after an exec — but not mouse tracking — so both
// handoffs re-issue EnableMouseCellMotion or every handoff kills the mouse
// until the program restarts.
func runHandoff(args []string) tea.Cmd {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "azrl"
	}
	c := exec.Command(self, args...)
	return tea.Sequence(tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return opDoneMsg{err: fmt.Errorf("%s exited: %w", args[0], err)}
		}
		return opDoneMsg{msg: fmt.Sprintf("%s complete", args[0])}
	}), tea.EnableMouseCellMotion)
}

// runShellHandoff suspends the TUI into `azrl … shell <name>` and reports the
// return neutrally: the subshell's own exit status is the user's business
// (their last command failing is not an azrl error).
func runShellHandoff(args []string) tea.Cmd {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "azrl"
	}
	c := exec.Command(self, args...)
	return tea.Sequence(tea.ExecProcess(c, func(error) tea.Msg {
		return opDoneMsg{msg: "shell exited"}
	}), tea.EnableMouseCellMotion)
}
