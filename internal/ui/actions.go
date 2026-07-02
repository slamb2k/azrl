package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// editorCmd resolves the user's preferred editor, honouring $VISUAL then
// $EDITOR and falling back to vi.
func editorCmd() string {
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

// runEdit suspends the TUI and opens the profile's .conf in $EDITOR, then
// resumes; the resulting opDoneMsg reloads the list so edits show immediately.
func runEdit(name string) tea.Cmd {
	conf := filepath.Join(config.ProfilesDir(), name+".conf")
	// pass the path as $1 so paths with spaces survive, while still allowing
	// $EDITOR to carry its own flags (e.g. "code --wait").
	c := exec.Command("sh", "-c", editorCmd()+` "$1"`, "sh", conf)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return opDoneMsg{err: fmt.Errorf("editor exited: %w", err)}
		}
		if _, verr := profile.LoadConf(name, config.ProfilesDir()); verr != nil {
			return opDoneMsg{err: fmt.Errorf("saved, but %s.conf looks malformed: %v", name, verr)}
		}
		return opDoneMsg{msg: fmt.Sprintf("edited %s", name)}
	})
}

// runRelabel changes a profile's display label. The slug (identity) is
// untouched, so no files move and no .azprofile pointers break.
func runRelabel(slug, label string) tea.Cmd {
	return func() tea.Msg {
		if err := profile.SetLabel(slug, config.ProfilesDir(), label); err != nil {
			return opDoneMsg{err: err}
		}
		disp := label
		if disp == "" {
			disp = slug
		}
		return opDoneMsg{msg: fmt.Sprintf("renamed %s → %s", slug, disp)}
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
		// init was removed in v0.7.0; login creates a profile on first use.
		return []string{"login"}
	case "c":
		return []string{"capture"}
	}
	return nil
}

// groupArgs builds a provider-group invocation, accounting for the promoted
// ghrl binary where the gh group's verbs sit at the top level.
func groupArgs(group string, rest ...string) []string {
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
