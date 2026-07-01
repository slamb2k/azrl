package ui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// providerAction is one entry in a provider tab's action pane. run mutates the
// view in place to reflect the action's outcome (status line, reloaded list).
type providerAction struct {
	key, label string
	run        func(v *providerTabView)
}

// providerTabView is the shared provider-tab component: a profile-list pane plus
// an action pane, with cursor/selection, active-pane focus, WindowSize handling,
// and responsive truncation. AWS, GCP, and GitHub embed it and differ only in
// their provider, pre-rendered header, and action set (GitHub adds a Switch
// action). Sign-in and new-profile actions are interactive and point the user at
// the CLI; use/switch/remove act on the profile store directly.
type providerTabView struct {
	prov      provider.Provider
	actions   []providerAction
	header    string
	profiles  []profile.Listed
	cursor    int
	focus     int
	actionCur int
	width     int
	height    int
	status    string
}

// newProviderTabView builds the shared view for prov with the given pre-rendered
// header and action set, loading the profile list up front.
func newProviderTabView(prov provider.Provider, header string, actions []providerAction) providerTabView {
	v := providerTabView{prov: prov, header: header, actions: actions}
	v.profiles, _ = v.prov.ListProfiles(v.prov.ProfilesDir())
	return v
}

func (v providerTabView) Init() tea.Cmd { return nil }

// update runs the shared list/pane navigation and action dispatch, returning the
// mutated view. The embedding types wrap it so their own concrete type
// round-trips through Bubble Tea's tea.Model.
func (v providerTabView) update(msg tea.Msg) (providerTabView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width, v.height = msg.Width, msg.Height
	case switchTabMsg:
		// The dashboard jumped to this tab for a specific profile; move the cursor
		// onto it so it's pre-selected. No-op when the profile isn't listed here.
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
				if v.actionCur < len(v.actions)-1 {
					v.actionCur++
				}
			} else if v.cursor < len(v.profiles)-1 {
				v.cursor++
			}
		case "enter":
			v = v.dispatch(v.actions[v.actionCur].key)
		default:
			// An accelerator key selects its action and runs it; any other key is a
			// no-op (arrows/tab/enter are handled above).
			for i, a := range v.actions {
				if a.key == msg.String() {
					v.actionCur = i
					v = v.dispatch(a.key)
					break
				}
			}
		}
	}
	return v, nil
}

// selected returns the highlighted profile slug, or "".
func (v providerTabView) selected() string {
	if v.cursor < 0 || v.cursor >= len(v.profiles) {
		return ""
	}
	return v.profiles[v.cursor].Name
}

// dispatch runs the action bound to key against the selected profile.
func (v providerTabView) dispatch(key string) providerTabView {
	for _, a := range v.actions {
		if a.key == key {
			if a.run != nil {
				a.run(&v)
			}
			break
		}
	}
	return v
}

// useAction pins the current directory to the selected profile. Shared by all
// providers.
func useAction(v *providerTabView) {
	name := v.selected()
	if name == "" {
		v.status = mutedStyle.Render("no profile selected")
		return
	}
	dir := v.prov.ProfilesDir()
	pwd, _ := os.Getwd()
	if err := v.prov.Use(name, dir, pwd); err != nil {
		v.status = failureStyle.Render(err.Error())
	} else {
		v.status = successStyle.Render(fmt.Sprintf("pinned this dir to %q", name))
	}
}

// removeAction deletes the selected profile and reloads the list. Shared by all
// providers.
func removeAction(v *providerTabView) {
	name := v.selected()
	if name == "" {
		v.status = mutedStyle.Render("no profile selected")
		return
	}
	dir := v.prov.ProfilesDir()
	pwd, _ := os.Getwd()
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

func (v providerTabView) View() string {
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
	for i, a := range v.actions {
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
	// Keep the two-pane body inside the frame at narrow widths.
	if v.width > 0 {
		lines := strings.Split(body, "\n")
		for i, l := range lines {
			lines[i] = truncateLine(l, max(1, v.width-4))
		}
		body = strings.Join(lines, "\n")
	}

	help := mutedStyle.Render("[ ]") + " tab · " + mutedStyle.Render("⇥") + " pane · " +
		mutedStyle.Render("↑↓") + " select · " + mutedStyle.Render("↵") + " run · " + mutedStyle.Render("q") + " quit"

	parts := []string{v.header, "", body, ""}
	if v.status != "" {
		parts = append(parts, v.status, "")
	}
	parts = append(parts, help)
	return frameStyle.Render(strings.Join(parts, "\n"))
}
