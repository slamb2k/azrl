package ui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

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
// their provider, pre-rendered header, and action set. Sign-in and new-profile
// actions are interactive and point the user at the CLI; use/remove act on the
// profile store directly.
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

// identityStrip mirrors the Azure Model's header: a ◆-accented provider
// descriptor on the left and this dir's pinned profile (or a muted "not linked"
// note) on the right, separated by the same dotted divider.
func (v providerTabView) identityStrip() string {
	left := accentStyle.Render("◆") + " " + v.header
	right := mutedStyle.Render("not linked to this dir")
	pwd, _ := os.Getwd()
	if name, err := v.prov.Resolve("", pwd); err == nil && name != "" {
		right = mutedStyle.Render("this dir → ") + accentStyle.Render(name)
	}
	return left + mutedStyle.Render("   ·   ") + right
}

func (v providerTabView) View() string {
	_, leftW, rightW, _ := (Model{width: v.width, height: v.height}).dims()

	left := renderProfilePane(v.profiles, v.cursor, v.focus == focusProfiles, leftW)

	// Render the provider's own action set as the shared radio group so the right
	// pane matches Azure's ◉/○ + [key] look exactly (dispatch is unchanged).
	opts := make([]radioOption, len(v.actions))
	for i, a := range v.actions {
		opts[i] = radioOption{label: a.label, key: a.key}
	}
	r := radio{options: opts, cursor: v.actionCur, focused: v.focus == focusActions}
	right := paneTitle("ACTION", v.focus == focusActions) + "\n\n" + r.view(rightW)

	help := mutedStyle.Render("↑↓ select · ⇥ pane · ↵ run · [ ] tab · q quit")
	return renderPaneFrame(v.width, v.height, v.identityStrip(), left, right, v.status, help)
}
