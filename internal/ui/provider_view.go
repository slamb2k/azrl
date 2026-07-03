package ui

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// providerAction is one entry in a provider tab's action pane. run mutates the
// view in place to reflect the action's outcome (status line, reloaded list)
// and may return a command (e.g. a runHandoff exec) for interactive flows.
type providerAction struct {
	key, label string
	hint       string // short muted description shown beside the label when it fits
	run        func(v *providerTabView) tea.Cmd
}

// providerTabView is the shared provider-tab component: a profile-list pane plus
// an action pane, with cursor/selection, active-pane focus, WindowSize handling,
// and responsive truncation. AWS, GCP, and GitHub embed it and differ only in
// their provider, pre-rendered header, and action set. Sign-in and new-profile
// actions are interactive and point the user at the CLI; use/remove act on the
// profile store directly.
type providerTabView struct {
	prov       provider.Provider
	actions    []providerAction
	header     string
	profiles   []profile.Listed
	statuses   map[string]provider.Status
	ambIdent   string
	active     string
	dirProfile string
	dirScope   string
	mapped     map[string]bool
	cursor     int
	focus      int
	actionCur  int
	width      int
	height     int
	status     string
	suspended  bool
	touched    bool
}

// newProviderTabView builds the shared view for prov with the given pre-rendered
// header and action set, loading the profile list up front.
func newProviderTabView(prov provider.Provider, header string, actions []providerAction) providerTabView {
	v := providerTabView{prov: prov, header: header, actions: actions}
	v.reload()
	return v
}

// reload refreshes the profile list and recomputes the row-icon inputs: which
// saved profile the current directory resolves to (with its pin's scope) and
// which one the provider's ambient identity matches (the global default).
// Disk-only, mirroring the dashboard's aggregation.
func (v *providerTabView) reload() {
	dir := v.prov.ProfilesDir()
	v.profiles, _ = v.prov.ListProfiles(dir)
	v.statuses = make(map[string]provider.Status, len(v.profiles))
	statuses := make([]provider.Status, 0, len(v.profiles))
	for _, p := range v.profiles {
		if st, err := v.prov.Status(p.Name, dir); err == nil {
			v.statuses[p.Name] = st
			statuses = append(statuses, st)
		}
	}
	v.active, v.ambIdent = "", ""
	if amb, err := v.prov.Ambient(); err == nil && amb.Identity != "" {
		v.ambIdent = amb.Identity
		v.active = provider.MatchProfile(statuses, amb.Identity)
	}
	v.mapped = map[string]bool{}
	for _, m := range v.prov.Scheme().ReadMappings(dir) {
		v.mapped[m.Profile] = true
	}
	v.dirProfile, v.dirScope = "", ""
	pwd, _ := os.Getwd()
	if name, err := v.prov.Resolve("", pwd); err == nil && name != "" {
		v.dirProfile = name
		v.dirScope = ScopeAncestor
		if pdir, ok := v.prov.Scheme().Locate(pwd); ok {
			if pdir == pwd {
				v.dirScope = ScopeCwd
			}
		} else if _, err := os.Stat(filepath.Join(pwd, ".git")); err == nil {
			// Resolved without a pointer (repo-local git config); the config
			// governs the whole repo, so the repo root counts as "this dir".
			v.dirScope = ScopeCwd
		}
	}
	if v.cursor >= len(v.profiles) {
		v.cursor = 0
	}
}

// visibleActions filters the action set for the current selection: actions
// that cannot apply are hidden (Use here when the selected profile already
// pins this directory).
func (v providerTabView) visibleActions() []providerAction {
	sel := v.selected()
	out := make([]providerAction, 0, len(v.actions))
	for _, a := range v.actions {
		if a.key == "u" && sel != "" && sel == v.dirProfile && v.dirScope == ScopeCwd {
			continue
		}
		out = append(out, a)
	}
	return out
}

// rowScope returns the effective relevance of one profile row — the closest
// wins: a directory pin (cwd or ancestor) outranks the global default, which
// outranks being mapped elsewhere; "" means mapped nowhere (deep-grey icon).
func (v providerTabView) rowScope(name string) string {
	if name == v.dirProfile {
		return v.dirScope
	}
	if name == v.active {
		return scopeGlobal
	}
	if v.mapped[name] {
		return scopeElsewhere
	}
	return ""
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
		if k := msg.String(); k != "q" && k != "ctrl+c" {
			// Any navigation marks the pane as visited (bold titles from here on).
			v.touched = true
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return v, tea.Quit
		case "right":
			v.focus = focusActions
		case "esc", "left":
			v.focus = focusProfiles
		case "up", "k":
			if v.focus == focusActions {
				if v.actionCur > 0 {
					v.actionCur--
				}
			} else if v.cursor > 0 {
				v.cursor--
				v.clampAction()
			} else {
				// Already at the top of the list: hand focus to the tab bar.
				return v, func() tea.Msg { return focusTabsMsg{} }
			}
		case "down", "j":
			if v.focus == focusActions {
				if v.actionCur < len(v.visibleActions())-1 {
					v.actionCur++
				}
			} else if v.cursor < len(v.profiles)-1 {
				v.cursor++
				v.clampAction()
			}
		case "enter":
			// Selecting a profile opens the action pane; enter there runs the action.
			if v.focus == focusProfiles {
				v.focus = focusActions
			} else if acts := v.visibleActions(); v.actionCur < len(acts) {
				return v.dispatch(acts[v.actionCur].key)
			}
		default:
			// An accelerator key selects its action and runs it; any other key is a
			// no-op (arrows/tab/enter are handled above). Hidden actions' keys are
			// inert for the current selection.
			for i, a := range v.visibleActions() {
				if a.key == msg.String() {
					v.actionCur = i
					return v.dispatch(a.key)
				}
			}
		}
	case barFocusMsg:
		v.suspended = msg.focused
		if !msg.focused {
			// Navigating down from the tab bar counts as entering the pane.
			v.touched = true
		}
	case cwdChangedMsg:
		// The header shows the directory; no bottom-bar echo needed.
		v.reload()
	case opDoneMsg:
		// An interactive handoff (sign in, new profile, adopt) finished; pick up
		// whatever it changed on disk.
		v.reload()
		if msg.err != nil {
			v.status = failureStyle.Render("✗ " + msg.err.Error())
		} else if msg.msg != "" {
			v.status = successStyle.Render("✓ " + msg.msg)
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
func (v providerTabView) dispatch(key string) (providerTabView, tea.Cmd) {
	for _, a := range v.actions {
		if a.key == key {
			if a.run != nil {
				return v, a.run(&v)
			}
			break
		}
	}
	return v, nil
}

// loginAction hands off to the provider's interactive `login` flow in the
// terminal (browser bridge and prompts included), signing into the selected
// profile — or the bare picker/create flow when withSelected is false.
func loginAction(group string, withSelected bool) func(v *providerTabView) tea.Cmd {
	return func(v *providerTabView) tea.Cmd {
		args := groupArgs(group, "login")
		if withSelected {
			if name := v.selected(); name != "" {
				args = append(args, name)
			}
		}
		v.status = ""
		return runHandoff(args)
	}
}

// useAction pins the current directory to the selected profile. Shared by all
// providers.
func useAction(v *providerTabView) tea.Cmd {
	name := v.selected()
	if name == "" {
		v.status = mutedStyle.Render("no profile selected")
		return nil
	}
	dir := v.prov.ProfilesDir()
	pwd, _ := os.Getwd()
	if err := v.prov.Use(name, dir, pwd); err != nil {
		v.status = failureStyle.Render(err.Error())
	} else {
		v.status = successStyle.Render(fmt.Sprintf("pinned this dir to %q", name))
	}
	return nil
}

// removeAction deletes the selected profile and reloads the list. Shared by all
// providers.
func removeAction(v *providerTabView) tea.Cmd {
	name := v.selected()
	if name == "" {
		v.status = mutedStyle.Render("no profile selected")
		return nil
	}
	dir := v.prov.ProfilesDir()
	pwd, _ := os.Getwd()
	if _, err := v.prov.Remove(name, dir, pwd); err != nil {
		v.status = failureStyle.Render(err.Error())
	} else {
		v.status = successStyle.Render(fmt.Sprintf("removed %q", name))
		v.reload()
	}
	return nil
}

// identityStrip is the standard provider header: icon + title, the current
// directory, and the effective identity there (the dir-pinned profile's
// identity, else the provider's ambient default).
func (v providerTabView) identityStrip() string {
	pwd, _ := os.Getwd()
	return headerStrip(providerIcon(v.prov.Name()), v.prov.Title(), pwd,
		effectiveIdentity(v.dirProfile, v.statuses[v.dirProfile].Identity, v.ambIdent))
}

func (v providerTabView) View() string {
	_, leftW, rightW, _ := (Model{width: v.width, height: v.height}).dims()

	scopes := make(map[string]string, len(v.profiles))
	for _, p := range v.profiles {
		scopes[p.Name] = v.rowScope(p.Name)
	}
	profMode := selNone
	switch {
	case v.suspended:
		// The tab bar holds focus: no selection below it.
	case v.focus == focusProfiles:
		profMode = selActive
	case v.focus == focusActions:
		profMode = selParent
	}
	left := renderProfilePane(v.profiles, v.cursor, profMode, v.touched, leftW, scopes)

	// DETAILS: the selected profile's info block, then its visible actions as
	// the shared radio group (keycaps left; selection block only when focused).
	acts := v.visibleActions()
	opts := make([]radioOption, len(acts))
	for i, a := range acts {
		opts[i] = radioOption{label: a.label, key: a.key, hint: a.hint}
	}
	r := radio{options: opts, cursor: v.actionCur, focused: v.focus == focusActions && !v.suspended}
	info := mutedStyle.Render("(no profile selected)")
	if v.cursor >= 0 && v.cursor < len(v.profiles) {
		pr := v.profiles[v.cursor]
		info = profileInfoBlock(pr, v.statuses[pr.Name], rightW)
	}
	actionsBody := r.view(rightW)
	if len(v.profiles) == 0 {
		actionsBody = mutedStyle.Render("(no profile selected)")
	}
	right := paneTitle("DETAILS", v.focus == focusActions) + "\n\n" +
		info + "\n\n" + rule(rightW) + "\n" +
		paneTitle(fmt.Sprintf("ACTIONS (%d)", len(acts)), v.focus == focusActions && !v.suspended) + "\n\n" + actionsBody

	help := mutedStyle.Render("↑↓ select · → details · ↵ open/run · esc back · ⇥ tab · ") +
		keycap("d") + mutedStyle.Render(" dir · ") + keycap("o") + mutedStyle.Render(" options · ") + keycap("q") + mutedStyle.Render(" quit")
	return renderPaneFrame(v.width, v.height, v.identityStrip(), left, right, scopeLegend(leftW), v.status, help)
}

// clampAction keeps the action cursor inside the selection's visible set.
func (v *providerTabView) clampAction() {
	if n := len(v.visibleActions()); v.actionCur >= n && n > 0 {
		v.actionCur = n - 1
	}
}
