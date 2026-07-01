package ui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// githubAction is one entry in the GitHub tab's action pane.
type githubAction struct{ key, label string }

var githubActions = []githubAction{
	{"s", "Sign in"},
	{"u", "Use here"},
	{"w", "Switch"},
	{"a", "New profile"},
	{"d", "Remove"},
}

// githubView is the GitHub provider tab: a profile list plus an action pane,
// mirroring the Azure tab's layout with the shared styles. Sign-in and new-
// profile are interactive (they run gh on a real terminal) and point the user at
// the CLI; use/switch/remove act on the profile store directly.
type githubView struct {
	prov      provider.Provider
	profiles  []profile.Listed
	cursor    int
	focus     int
	actionCur int
	width     int
	height    int
	status    string
}

func newGithubView() githubView {
	v := githubView{prov: github.NewProvider()}
	v.profiles, _ = v.prov.ListProfiles(v.prov.ProfilesDir())
	return v
}

func (v githubView) Init() tea.Cmd { return nil }

func (v githubView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width, v.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return v, tea.Quit
		case "tab", "shift+tab":
			v.focus = focusActions - v.focus
		case "left":
			v.focus = focusProfiles
		case "right":
			v.focus = focusActions
		case "up", "k":
			if v.focus == focusActions {
				if v.actionCur > 0 {
					v.actionCur--
				}
			} else if v.cursor > 0 {
				v.cursor--
			}
		case "down", "j":
			if v.focus == focusActions {
				if v.actionCur < len(githubActions)-1 {
					v.actionCur++
				}
			} else if v.cursor < len(v.profiles)-1 {
				v.cursor++
			}
		case "enter":
			v = v.dispatch(githubActions[v.actionCur].key)
		case "s", "u", "w", "a", "d":
			for i, a := range githubActions {
				if a.key == msg.String() {
					v.actionCur = i
				}
			}
			v = v.dispatch(msg.String())
		}
	}
	return v, nil
}

// selected returns the highlighted profile slug, or "".
func (v githubView) selected() string {
	if v.cursor < 0 || v.cursor >= len(v.profiles) {
		return ""
	}
	return v.profiles[v.cursor].Name
}

// dispatch runs an action against the selected profile and updates status.
func (v githubView) dispatch(key string) githubView {
	dir := v.prov.ProfilesDir()
	pwd, _ := os.Getwd()
	name := v.selected()
	switch key {
	case "s":
		v.status = accentStyle.Render("Run `ghrl login` in a terminal to sign in (interactive).")
	case "a":
		v.status = accentStyle.Render("Run `ghrl login <name>` to create and sign into a new profile.")
	case "u":
		if name == "" {
			v.status = mutedStyle.Render("no profile selected")
			break
		}
		if err := v.prov.Use(name, dir, pwd); err != nil {
			v.status = failureStyle.Render(err.Error())
		} else {
			v.status = successStyle.Render(fmt.Sprintf("pinned this dir to %q", name))
		}
	case "w":
		if name == "" {
			v.status = mutedStyle.Render("no profile selected")
			break
		}
		if err := github.Switch(dir, name); err != nil {
			v.status = failureStyle.Render(err.Error())
		} else {
			v.status = successStyle.Render(fmt.Sprintf("switched active profile to %q", name))
		}
	case "d":
		if name == "" {
			v.status = mutedStyle.Render("no profile selected")
			break
		}
		if _, err := v.prov.Remove(name, dir, pwd); err != nil {
			v.status = failureStyle.Render(err.Error())
		} else {
			v.status = successStyle.Render(fmt.Sprintf("removed %q", name))
			v.profiles, _ = v.prov.ListProfiles(dir)
			if v.cursor >= len(v.profiles) {
				v.cursor = 0
			}
		}
	}
	return v
}

func (v githubView) View() string {
	var left strings.Builder
	fmt.Fprintln(&left, paneTitleStyle.Render(fmt.Sprintf("PROFILES (%d)", len(v.profiles))))
	if len(v.profiles) == 0 {
		fmt.Fprintln(&left, mutedStyle.Render("  (none yet — Sign in to add one)"))
	}
	for i, p := range v.profiles {
		marker := "  "
		name := p.Display()
		if i == v.cursor {
			if v.focus == focusProfiles {
				marker = accentStyle.Render("› ")
				name = selectedStyle.Render(name)
			} else {
				marker = dotStyle.Render("• ")
			}
		}
		fmt.Fprintf(&left, "%s%-16s %s\n", marker, name, mutedStyle.Render(p.Detail))
	}

	var right strings.Builder
	fmt.Fprintln(&right, paneTitleStyle.Render("ACTION"))
	for i, a := range githubActions {
		marker := "  "
		label := a.label
		if i == v.actionCur {
			if v.focus == focusActions {
				marker = accentStyle.Render("› ")
				label = selectedStyle.Render(label)
			} else {
				marker = dotStyle.Render("• ")
			}
		}
		fmt.Fprintf(&right, "%s%s %s\n", marker, keycapStyle.Render("["+a.key+"]"), label)
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(40).Render(left.String()),
		dividerStyle.Render(" │ "),
		right.String(),
	)

	header := paneTitleStyle.Render("GitHub") + mutedStyle.Render(" — github.com · *.ghe.com · GHES")
	help := mutedStyle.Render("[ ]") + " tab · " + mutedStyle.Render("⇥") + " pane · " +
		mutedStyle.Render("↑↓") + " select · " + mutedStyle.Render("↵") + " run · " + mutedStyle.Render("q") + " quit"

	parts := []string{header, "", body, ""}
	if v.status != "" {
		parts = append(parts, v.status, "")
	}
	parts = append(parts, help)
	return frameStyle.Render(strings.Join(parts, "\n"))
}
