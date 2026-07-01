package ui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// awsAction is one entry in the AWS tab's action pane.
type awsAction struct{ key, label string }

var awsActions = []awsAction{
	{"s", "Sign in"},
	{"u", "Use here"},
	{"a", "New profile"},
	{"d", "Remove"},
}

// awsView is the AWS provider tab: a profile list plus an action pane, mirroring
// the GitHub tab's layout. AWS has no active-profile file (only a cwd pin plus
// the ambient AWS_PROFILE), so there is no Switch action. Sign-in and new-profile
// are interactive (they run aws on a real terminal) and point the user at the
// CLI; use/remove act on the profile store directly.
type awsView struct {
	prov      provider.Provider
	profiles  []profile.Listed
	cursor    int
	focus     int
	actionCur int
	width     int
	height    int
	status    string
}

func newAwsView() awsView {
	v := awsView{prov: aws.NewProvider()}
	v.profiles, _ = v.prov.ListProfiles(v.prov.ProfilesDir())
	return v
}

func (v awsView) Init() tea.Cmd { return nil }

func (v awsView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width, v.height = msg.Width, msg.Height
	case switchTabMsg:
		for i, p := range v.profiles {
			if p.Name == msg.profile {
				v.cursor = i
				break
			}
		}
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
				if v.actionCur < len(awsActions)-1 {
					v.actionCur++
				}
			} else if v.cursor < len(v.profiles)-1 {
				v.cursor++
			}
		case "enter":
			v = v.dispatch(awsActions[v.actionCur].key)
		case "s", "u", "a", "d":
			for i, a := range awsActions {
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
func (v awsView) selected() string {
	if v.cursor < 0 || v.cursor >= len(v.profiles) {
		return ""
	}
	return v.profiles[v.cursor].Name
}

// dispatch runs an action against the selected profile and updates status.
func (v awsView) dispatch(key string) awsView {
	dir := v.prov.ProfilesDir()
	pwd, _ := os.Getwd()
	name := v.selected()
	switch key {
	case "s":
		v.status = accentStyle.Render("Run `azrl aws login` in a terminal to sign in (interactive).")
	case "a":
		v.status = accentStyle.Render("Run `azrl aws login <name>` to create and sign into a new profile.")
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

func (v awsView) View() string {
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
	for i, a := range awsActions {
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

	leftW := 40
	if v.width > 0 {
		leftW = min(40, max(20, v.width/2))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftW).Render(left.String()),
		dividerStyle.Render(" │ "),
		right.String(),
	)
	if v.width > 0 {
		lines := strings.Split(body, "\n")
		for i, l := range lines {
			lines[i] = truncateLine(l, max(1, v.width-4))
		}
		body = strings.Join(lines, "\n")
	}

	header := paneTitleStyle.Render("AWS") + mutedStyle.Render(" — IAM Identity Center · SSO")
	help := mutedStyle.Render("[ ]") + " tab · " + mutedStyle.Render("⇥") + " pane · " +
		mutedStyle.Render("↑↓") + " select · " + mutedStyle.Render("↵") + " run · " + mutedStyle.Render("q") + " quit"

	parts := []string{header, "", body, ""}
	if v.status != "" {
		parts = append(parts, v.status, "")
	}
	parts = append(parts, help)
	return frameStyle.Render(strings.Join(parts, "\n"))
}
