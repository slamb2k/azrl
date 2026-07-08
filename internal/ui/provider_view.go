package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"

	"github.com/slamb2k/azrl/internal/browserpick"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// focus identifies which pane receives navigation keys.
const (
	focusProfiles = iota
	focusActions
)

// providerAction is one entry in a provider tab's action pane. run mutates the
// view in place to reflect the action's outcome (status line, reloaded list)
// and may return a command (e.g. a runHandoff exec) for interactive flows.
type providerAction struct {
	key, label string
	hint       string // short muted description shown beside the label when it fits
	run        func(v *providerTabView) tea.Cmd
	bootstrap  bool // offered in the empty state (onboarding verbs)
}

// actionState is one action resolved against the current selection: always
// listed; disabled (with the reason swapped into the hint) when it can't apply.
type actionState struct {
	providerAction
	enabled bool
}

// providerTabView is the shared provider-tab component: a profile-list pane plus
// an action pane, with cursor/selection, active-pane focus, WindowSize handling,
// and responsive truncation. AWS, GCP, and GitHub embed it and differ only in
// their provider and CLI command group. Sign-in and new-profile actions are
// interactive and point the user at the CLI; use/remove act on the profile
// store directly.
type providerTabView struct {
	prov        provider.Provider
	actions     []providerAction
	profiles    []profile.Listed
	statuses    map[string]provider.Status
	ambIdent    string
	active      string
	dirProfile  string
	dirScope    string
	mappingDirs map[string][]string
	cursor      int
	focus       int
	actionCur   int
	width       int
	height      int
	status      string
	suspended   bool
	touched     bool
	namingVerb  string // "" (not naming), "login", or "capture"
	nameInput   textinput.Model
	shellName   string // set from AZRL_PROFILE when its provider prefix matches this tab

	confirming     bool
	pendingDelete  string
	confirm        radio
	confirmKind    []string // one of "cancel"/"remove"/"unlink"/"replace" per v.confirm option, in order
	linkedDirs     []string // dirs LinkedDirs found for pendingDelete when it was armed; nil for a link-free profile
	replacePicking bool     // second-level picker: choosing which other profile to repoint linkedDirs at
	replacePick    radio

	// notice is an optional extra header line (e.g. Azure's drift warning);
	// identityOverride, when set, replaces the dir-linked profile's disk
	// identity in the header (Azure's live az-account-show result is fresher).
	notice           string
	identityOverride string

	browserFor    string // profile a browser mapping is being chosen for
	browserPick   *browserPicker
	browserManual bool
	browserInput  textinput.Model
}

// providerActions is the shared verb set every tab offers. group is the azrl
// CLI command group the interactive verbs exec ("gh" for GitHub, "" for
// Azure's top-level verbs).
func providerActions(group string) []providerAction {
	return []providerAction{
		{key: "s", label: "Renew", hint: "sign in again — mappings unchanged", run: loginAction(group)},
		{key: "t", label: "Shell as…", hint: "subshell as this profile — no mapping", run: shellAction},
		{key: "n", label: "New profile", hint: "token container + sign-in — map it later", run: newProfileAction, bootstrap: true},
		{key: "a", label: "Capture session", hint: "adopt current CLI session · maps this dir", run: captureAction, bootstrap: true},
		{key: "b", label: "Assign browser…", hint: "map to a local browser profile", run: browserAction},
		{key: "c", label: "Open console", hint: "web console as this credential", run: consoleAction},
		{key: "delete", label: "Delete…", hint: "delete profile", run: removeAction},
	}
}

// newProviderTabView builds the shared view for prov with the given action set,
// loading the profile list up front.
func newProviderTabView(prov provider.Provider, actions []providerAction) providerTabView {
	zone.NewGlobal() // idempotent — defensive so tests constructing a view directly still have a manager.
	v := providerTabView{prov: prov, actions: actions}
	v.reload()
	return v
}

// reload refreshes the profile list and recomputes the row-icon inputs: which
// saved profile the current directory resolves to (with its link's scope) and
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
	v.mappingDirs = map[string][]string{}
	for _, m := range v.prov.Scheme().ReadMappings(dir) {
		v.mappingDirs[m.Profile] = append(v.mappingDirs[m.Profile], m.Dir)
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
	// v.cursor indexes v.profiles directly; the NEW ＋ affordance lives in the
	// pane title, not the list.
	if v.cursor >= len(v.profiles) {
		v.cursor = 0
	}
	v.shellName = ""
	if ov := os.Getenv("AZRL_PROFILE"); ov != "" {
		if prov, prof, ok := strings.Cut(ov, ":"); ok && prov == v.prov.Name() {
			v.shellName = prof
		}
	}
}

// enabledActions resolves the action set against the current selection.
// Nothing is ever hidden: a verb that can't apply is listed disabled with its
// reason as the hint. The empty state is the one exception — only the
// bootstrap (onboarding) verbs show.
func (v providerTabView) enabledActions() []actionState {
	if len(v.profiles) == 0 {
		var out []actionState
		for _, a := range v.actions {
			if a.key == "n" {
				// New profile is the PROFILES pane's title-bar NEW ＋ affordance,
				// not a listed verb — even in the empty state.
				continue
			}
			if a.bootstrap {
				out = append(out, actionState{providerAction: a, enabled: true})
			}
		}
		return out
	}
	sel := v.selected()
	out := make([]actionState, 0, len(v.actions))
	for _, a := range v.actions {
		if a.key == "a" || a.key == "n" {
			// Capture is onboarding-contextual (empty state + dashboard adopt
			// only); New profile lives as the PROFILES title-bar NEW ＋ affordance.
			continue
		}
		st := actionState{providerAction: a, enabled: true}
		if a.key == "s" && sessionLive(v.statuses[sel]) {
			// Still runnable — re-auth is idempotent — but say why it's optional.
			st.hint = "session live · re-auth anyway"
		}
		out = append(out, st)
	}
	return out
}

// rowScope returns one profile row's relevance to the current directory —
// the closest link wins; the global default is tagged, not scoped; anything
// else renders an empty slot.
func (v providerTabView) rowScope(name string) string {
	if name == v.dirProfile {
		return v.dirScope
	}
	if name == v.active {
		return scopeGlobal
	}
	return ""
}

func (v providerTabView) Init() tea.Cmd { return nil }

// capturesInput reports whether the new-profile name input is active, so the
// container forwards keys here instead of acting on them.
func (v providerTabView) capturesInput() bool {
	return v.namingVerb != "" || v.browserManual || v.browserPick != nil || v.confirming
}

// cliGroup maps a provider name to its azrl command group ("" = the verbs
// sit at the top level, as Azure's do).
func cliGroup(name string) string {
	switch name {
	case "github":
		return "gh"
	case "azure":
		return ""
	}
	return name
}

// sessionLive reports whether a profile's disk-only status shows a usable
// session — identity present and not expired.
func sessionLive(st provider.Status) bool {
	return provider.SessionLive(st)
}

// update runs the shared list/pane navigation and action dispatch, returning the
// mutated view. The embedding types wrap it so their own concrete type
// round-trips through Bubble Tea's tea.Model.
func (v providerTabView) update(msg tea.Msg) (providerTabView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width, v.height = msg.Width, msg.Height
	case tea.MouseMsg:
		return v.handleMouse(msg)
	case switchTabMsg:
		// The dashboard jumped to this tab for a specific profile; move the cursor
		// onto it so it's pre-selected. No-op when the profile isn't listed here.
		for i, p := range v.profiles {
			if p.Name == msg.profile {
				v.cursor = i
				break
			}
		}
		// An accelerator action (e.g. "b" for browser profile) rides along: run it
		// against the now-selected profile and return its cmd so async flows (like
		// browser discovery) actually execute.
		if msg.action != "" {
			return v.dispatch(msg.action)
		}
	case tea.KeyMsg:
		if v.confirming && v.replacePicking {
			switch msg.String() {
			case "ctrl+c":
				return v, tea.Quit
			case "esc":
				v.cancelConfirm()
			case "up", "k", "left":
				v.replacePick.up()
			case "down", "j", "right":
				v.replacePick.down()
			case "enter":
				return v.doReplace(v.replacePick.selected().label)
			}
			return v, nil
		}
		if v.confirming {
			switch msg.String() {
			case "ctrl+c":
				return v, tea.Quit
			case "esc", "n", "q":
				v.cancelConfirm()
			case "y":
				// Fast path: link-free keeps the historical bare delete; a linked
				// profile maps 'y' to unlink-all so it can't strand a pointer file.
				if len(v.linkedDirs) > 0 {
					return v.doUnlinkAndRemove()
				}
				return v.doRemove()
			case "up", "k", "left":
				v.confirm.up()
			case "down", "j", "right":
				v.confirm.down()
			case "enter":
				return v.runConfirmSelection()
			}
			return v, nil
		}
		if v.browserPick != nil {
			np, picked, closed := v.browserPick.update(msg)
			v.browserPick = &np
			if closed {
				v.browserPick = nil
				if picked != nil {
					v.applyBrowserMapping(picked.Command(), picked.Label())
				} else {
					v.status = ""
				}
				v.browserFor = ""
			}
			return v, nil
		}
		if v.browserManual {
			switch msg.String() {
			case "esc":
				v.browserManual = false
				v.browserFor = ""
				v.status = ""
			case "enter":
				if c := strings.TrimSpace(v.browserInput.Value()); c != "" {
					v.browserManual = false
					v.applyBrowserMapping(c, "")
					v.browserFor = ""
				}
			default:
				var cmd tea.Cmd
				v.browserInput, cmd = v.browserInput.Update(msg)
				_ = cmd
			}
			return v, nil
		}
		if v.namingVerb != "" {
			switch msg.String() {
			case "esc":
				v.namingVerb = ""
			case "enter":
				name := strings.TrimSpace(v.nameInput.Value())
				if name == "" {
					name = v.nameInput.Placeholder
				}
				name = profile.SanitizeName(name)
				if name == "" {
					return v, nil
				}
				verb := v.namingVerb
				v.namingVerb = ""
				v.status = ""
				if verb == "capture" {
					return v, runHandoff(groupArgs(cliGroup(v.prov.Name()), "capture", name))
				}
				// Created from the pinned PROFILES row: sign in, but don't link
				// this directory — link is a separate, explicit act now.
				return v, runHandoff(groupArgs(cliGroup(v.prov.Name()), "login", name, "--yes", "--no-map"))
			default:
				var cmd tea.Cmd
				v.nameInput, cmd = v.nameInput.Update(msg)
				_ = cmd
			}
			return v, nil
		}
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
				if v.actionCur < len(v.enabledActions())-1 {
					v.actionCur++
				}
			} else if v.cursor < len(v.profiles)-1 {
				v.cursor++
				v.clampAction()
			}
		case "enter":
			// Enter on a profile opens the action pane, where enter runs the action.
			if v.focus == focusProfiles {
				v.focus = focusActions
			} else if acts := v.enabledActions(); v.actionCur < len(acts) {
				a := acts[v.actionCur]
				if !a.enabled {
					v.status = mutedStyle.Render(a.hint)
					return v, nil
				}
				return v.dispatch(a.key)
			}
		case "n":
			// New profile always opens the naming prompt, from any row or focus —
			// it has no listed ACTIONS entry (the title-bar NEW ＋ is its home).
			return v.dispatch("n")
		case "f5", "r":
			v.reload()
		default:
			// An accelerator key selects its action and runs it; a disabled
			// action's key explains itself in the status line instead.
			return v.selectAndRun(msg.String())
		}
	case barFocusMsg:
		v.suspended = msg.focused
		if !msg.focused {
			// Navigating down from the tab bar counts as entering the pane.
			v.touched = true
		}
	case browserProfilesMsg:
		if msg.forProfile != v.browserFor || v.browserFor == "" {
			return v, nil
		}
		if v.confirming || v.namingVerb != "" {
			// The user armed another sub-state while discovery was in flight;
			// don't stack the picker/manual-entry prompt on top of it.
			v.browserFor = ""
			return v, nil
		}
		if msg.err != nil || len(msg.profiles) == 0 {
			ti := textinput.New()
			ti.Placeholder = "e.g. microsoft-edge --profile-directory=\"Profile 2\""
			ti.Focus()
			v.browserInput = ti
			v.browserManual = true
			v.status = mutedStyle.Render("discovery unavailable — enter a command")
			return v, nil
		}
		ident := v.statuses[v.browserFor].Identity
		pk := newBrowserPicker(msg.profiles, ident)
		v.browserPick = &pk
		return v, nil
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

// handleMouse resolves a mouse event against the view's own zones: wheel
// scrolls the profile cursor (clamped, never handing focus to the tab bar —
// unlike ↑ at the top row), left-release hit-tests the profile and action row
// zones marked in renderProfilePane/radio.view. The browser picker overlay
// gets its own routing (handleBrowserPickMouse) since it owns input while
// open; every other sub-state (naming prompt, confirm dialog, manual browser
// entry) makes mouse events a no-op.
func (v providerTabView) handleMouse(msg tea.MouseMsg) (providerTabView, tea.Cmd) {
	if v.browserPick != nil {
		return v.handleBrowserPickMouse(msg)
	}
	if v.capturesInput() {
		return v, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		if v.cursor < len(v.profiles)-1 {
			v.cursor++
			v.clampAction()
		}
		return v, nil
	case tea.MouseButtonWheelUp:
		if v.cursor > 0 {
			v.cursor--
			v.clampAction()
		}
		return v, nil
	}
	if !leftRelease(msg) {
		return v, nil
	}
	if z := zone.Get("prof:+new"); z != nil && z.InBounds(msg) {
		// The title-bar NEW ＋ affordance is a button, not a row: one click
		// opens the naming prompt directly.
		return v.dispatch("n")
	}
	for _, p := range v.profiles {
		if z := zone.Get("prof:" + p.Name); z != nil && z.InBounds(msg) {
			return v.clickProfile(p.Name)
		}
	}
	for _, a := range v.enabledActions() {
		if z := zone.Get("act:" + a.key); z != nil && z.InBounds(msg) {
			return v.clickAction(a.key)
		}
	}
	return v, nil
}

// clickProfile handles a click on a profile row: selecting a different row
// moves the cursor there; clicking the already-selected row is the same as
// pressing enter on the profiles pane (focus moves to actions).
func (v providerTabView) clickProfile(name string) (providerTabView, tea.Cmd) {
	if v.selected() == name {
		v.focus = focusActions
		return v, nil
	}
	for i, p := range v.profiles {
		if p.Name == name {
			v.cursor = i
			v.focus = focusProfiles
			v.clampAction()
			break
		}
	}
	return v, nil
}

// clickAction handles a click on an action row — mirrors the accelerator-key
// loop exactly: select the row, then run it if enabled, else surface its
// disabled reason in the status line.
func (v providerTabView) clickAction(key string) (providerTabView, tea.Cmd) {
	return v.selectAndRun(key)
}

// selectAndRun selects the action row matching key and runs it if enabled,
// else surfaces its disabled reason in the status line — the accelerator-key
// default case and clickAction are both thin wrappers over this so they stay
// byte-for-byte in sync.
func (v providerTabView) selectAndRun(key string) (providerTabView, tea.Cmd) {
	for i, a := range v.enabledActions() {
		if a.key == key {
			v.actionCur = i
			if !a.enabled {
				v.status = mutedStyle.Render(a.hint)
				return v, nil
			}
			return v.dispatch(a.key)
		}
	}
	return v, nil
}

// selected returns the highlighted profile slug, or "" on an empty list or
// an out-of-range cursor.
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

// loginAction hands off to the provider's interactive login flow for the selected profile (browser bridge included) — the recovery verb.
func loginAction(group string) func(v *providerTabView) tea.Cmd {
	return func(v *providerTabView) tea.Cmd {
		args := groupArgs(group, "login")
		if name := v.selected(); name != "" {
			args = append(args, name)
		}
		v.status = ""
		return runHandoff(args)
	}
}

// shellAction hands off to a subshell impersonating the selected profile —
// exports its env for the shell's lifetime, no mapping recorded.
func shellAction(v *providerTabView) tea.Cmd {
	name := v.selected()
	if name == "" {
		return nil
	}
	args := append(groupArgs(cliGroup(v.prov.Name()), "shell"), name)
	return runShellHandoff(args)
}

// consoleAction hands off to the provider's web console deep link for the
// selected profile — returns immediately, no session mutation.
func consoleAction(v *providerTabView) tea.Cmd {
	name := v.selected()
	if name == "" {
		return nil
	}
	args := append(groupArgs(cliGroup(v.prov.Name()), "console"), name)
	return runHandoff(args)
}

// namingPromptAction opens the name input with the given verb ("login" or "capture").
func namingPromptAction(verb string) func(v *providerTabView) tea.Cmd {
	return func(v *providerTabView) tea.Cmd {
		pwd, _ := os.Getwd()
		ti := textinput.New()
		ti.Placeholder = profile.DefaultName("", pwd)
		ti.Focus()
		v.nameInput = ti
		v.namingVerb = verb
		v.status = ""
		return nil
	}
}

// newProfileAction opens the name prompt; confirming execs `login <name>
// --yes --no-link`, whose create path signs in but leaves linking for later.
func newProfileAction(v *providerTabView) tea.Cmd {
	return namingPromptAction("login")(v)
}

// captureAction opens the name prompt for adopting the CLI's current session;
// confirming execs `capture <name>`, which records the session's metadata as
// a profile and links the current directory. Onboarding-contextual: offered
// only in the empty state (and via the dashboard's adopt flow).
func captureAction(v *providerTabView) tea.Cmd {
	return namingPromptAction("capture")(v)
}

// applyBrowserMapping writes the browser cmd/label keys for browserFor.
func (v *providerTabView) applyBrowserMapping(cmdVal, labelVal string) {
	cmdKey, labelKey := browserpick.Keys(v.prov.Name())
	s := v.prov.Scheme()
	dir := v.prov.ProfilesDir()
	if err := s.SetKey(v.browserFor, dir, cmdKey, cmdVal); err != nil {
		v.status = failureStyle.Render(err.Error())
		return
	}
	if err := s.SetKey(v.browserFor, dir, labelKey, labelVal); err != nil {
		v.status = failureStyle.Render(err.Error())
		return
	}
	disp := labelVal
	if disp == "" {
		disp = cmdVal
	}
	v.status = successStyle.Render(fmt.Sprintf("%q opens with %s", v.browserFor, disp))
}

// removeAction arms the shared confirm dialog for the selected profile —
// nothing is deleted until the user confirms. A profile with linked dirs gets
// a third option (besides Cancel) to unlink everywhere or repoint the links
// at another profile before deleting; a link-free profile keeps the original
// two-option Yes/No dialog untouched.
func removeAction(v *providerTabView) tea.Cmd {
	name := v.selected()
	if name == "" {
		v.status = mutedStyle.Render("no profile selected")
		return nil
	}
	dir := v.prov.ProfilesDir()
	v.confirming = true
	v.pendingDelete = name
	v.replacePicking = false
	v.linkedDirs = v.prov.Scheme().LinkedDirs(dir, name)
	if len(v.linkedDirs) == 0 {
		v.confirm = newRadio([]radioOption{
			{label: "No, keep it"},
			{label: "Yes, remove " + name},
		})
		v.confirmKind = []string{"cancel", "remove"}
	} else {
		otherProfiles := 0
		for _, p := range v.profiles {
			if p.Name != name {
				otherProfiles++
			}
		}
		unit := "dir"
		if len(v.linkedDirs) != 1 {
			unit = "dirs"
		}
		replaceOpt := radioOption{label: "Replace mappings with…"}
		if otherProfiles == 0 {
			replaceOpt.disabled = true
			replaceOpt.hint = "no other profile to point them at"
		}
		v.confirm = newRadio([]radioOption{
			{label: "Cancel"},
			{label: fmt.Sprintf("Unmap %d %s + delete", len(v.linkedDirs), unit)},
			replaceOpt,
		})
		v.confirmKind = []string{"cancel", "unlink", "replace"}
	}
	v.confirm.focused = true
	return nil
}

// cancelConfirm closes the confirm dialog (and any nested replace picker)
// without touching disk — the single exit used by esc/n/q at either level.
func (v *providerTabView) cancelConfirm() {
	v.confirming = false
	v.replacePicking = false
	v.pendingDelete = ""
	v.linkedDirs = nil
}

// runConfirmSelection dispatches enter on the top-level confirm radio by the
// semantic kind of the selected row (confirmKind), not its cursor index —
// the option set differs between the 2-option and 3-option dialogs.
func (v providerTabView) runConfirmSelection() (providerTabView, tea.Cmd) {
	kind := "cancel"
	if v.confirm.cursor < len(v.confirmKind) {
		kind = v.confirmKind[v.confirm.cursor]
	}
	switch kind {
	case "remove":
		return v.doRemove()
	case "unlink":
		return v.doUnlinkAndRemove()
	case "replace":
		if v.confirm.selected().disabled {
			v.status = mutedStyle.Render(v.confirm.selected().hint)
			return v, nil
		}
		return v.openReplacePicker()
	default:
		v.cancelConfirm()
		return v, nil
	}
}

// openReplacePicker arms the second-level radio listing every other profile
// as a repoint target.
func (v providerTabView) openReplacePicker() (providerTabView, tea.Cmd) {
	var opts []radioOption
	for _, p := range v.profiles {
		if p.Name != v.pendingDelete {
			opts = append(opts, radioOption{label: p.Name})
		}
	}
	v.replacePick = newRadio(opts)
	v.replacePick.focused = true
	v.replacePicking = true
	return v, nil
}

// doRemove deletes the pending profile and reloads the list.
func (v providerTabView) doRemove() (providerTabView, tea.Cmd) {
	name := v.pendingDelete
	v.cancelConfirm()
	dir := v.prov.ProfilesDir()
	pwd, _ := os.Getwd()
	if _, err := v.prov.Remove(name, dir, pwd); err != nil {
		v.status = failureStyle.Render(err.Error())
	} else {
		v.status = successStyle.Render(fmt.Sprintf("removed %q", name))
		v.reload()
		v.clampAction()
	}
	return v, nil
}

// doUnlinkAndRemove drops every linked dir's pointer + mapping row, then
// deletes the profile itself.
func (v providerTabView) doUnlinkAndRemove() (providerTabView, tea.Cmd) {
	name := v.pendingDelete
	dir := v.prov.ProfilesDir()
	if _, err := v.prov.Scheme().UnlinkAll(dir, name); err != nil {
		v.status = failureStyle.Render(err.Error())
		v.cancelConfirm()
		return v, nil
	}
	return v.doRemove()
}

// doReplace repoints every linked dir at newName, then deletes the original
// profile — the dir governed by pwd (if any) now names newName, so
// prov.Remove no longer finds it a target and leaves it alone.
func (v providerTabView) doReplace(newName string) (providerTabView, tea.Cmd) {
	name := v.pendingDelete
	dir := v.prov.ProfilesDir()
	if _, err := v.prov.Scheme().ReplaceLinks(dir, name, newName); err != nil {
		v.status = failureStyle.Render(err.Error())
		v.cancelConfirm()
		return v, nil
	}
	return v.doRemove()
}

// formatDirList renders up to the first 3 dirs one per line, display-shortened,
// plus a "+ n more" trailer when there are more.
func formatDirList(dirs []string) string {
	shown := dirs
	if len(dirs) > 3 {
		shown = dirs[:3]
	}
	lines := make([]string, len(shown))
	for i, d := range shown {
		lines[i] = "  " + displayDir(d)
	}
	text := strings.Join(lines, "\n")
	if len(dirs) > 3 {
		text += fmt.Sprintf("\n  + %d more", len(dirs)-3)
	}
	return text
}

// identityStrip is the standard provider header: icon + title, the current
// directory, the effective identity there, and an optional notice line.
func (v providerTabView) identityStrip() string {
	pwd, _ := os.Getwd()
	contentW, _, _ := paneDims(v.width)
	dirIdentity := v.statuses[v.dirProfile].Identity
	if v.identityOverride != "" {
		dirIdentity = v.identityOverride
	}
	ident := effectiveIdentity(v.dirProfile, dirIdentity, v.ambIdent)
	if v.shellName != "" {
		ident = accentStyle.Render("⌁ shell: " + v.shellName)
	}
	strip := headerStrip(contentW, providerIcon(v.prov.Name()), v.prov.Title(), pwd, ident)
	if v.notice != "" {
		strip += "\n" + ansi.Wordwrap(v.notice, contentW, "")
	}
	return strip
}

func (v providerTabView) View() string {
	_, leftW, rightW := paneDims(v.width)

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

	// DETAILS: the selected profile's info block, then its enabled actions as
	// the shared radio group (keycaps left; selection block only when focused).
	acts := v.enabledActions()
	opts := make([]radioOption, len(acts))
	for i, a := range acts {
		opts[i] = radioOption{label: a.label, key: a.key, hint: a.hint, disabled: !a.enabled}
	}
	r := radio{options: opts, cursor: v.actionCur, focused: v.focus == focusActions && !v.suspended}
	info := mutedStyle.Render("(no profile selected)")
	if v.cursor >= 0 && v.cursor < len(v.profiles) {
		pr := v.profiles[v.cursor]
		st := v.statuses[pr.Name]
		note := ""
		if st.Drifted {
			note = "shell uses " + orNoSession(v.ambIdent)
		}
		cmdKey, labelKey := browserpick.Keys(v.prov.Name())
		browser := v.prov.Scheme().GetKey(pr.Name, v.prov.ProfilesDir(), labelKey)
		if browser == "" {
			browser = v.prov.Scheme().GetKey(pr.Name, v.prov.ProfilesDir(), cmdKey)
		}
		linked := ""
		if dirs := v.mappingDirs[pr.Name]; len(dirs) > 0 {
			linked = displayDir(dirs[0])
			if len(dirs) > 1 {
				linked += fmt.Sprintf(" + %d more", len(dirs)-1)
			}
		}
		info = profileInfoBlock(v.prov.Name(), pr, st, browser, linked, note, rightW)
	}
	actionsBody := r.view(rightW)
	if v.namingVerb != "" {
		prompt, confirmHint := "Name for the new profile:", "token container + sign-in — map it later"
		// The create prompt carries the entity blurb — the education moment now
		// that NEW ＋ is a title-bar button with no selectable row of its own.
		intro := mutedStyle.Render(ansi.Wordwrap("A profile is a container for tokens and intention — sign in once, map it to any number of directories.", rightW, "")) + "\n\n"
		if v.namingVerb == "capture" {
			prompt, confirmHint = "Name for the captured profile:", "adopt session + link"
			intro = ""
		}
		actionsBody = intro + mutedStyle.Render(prompt) + "\n\n" +
			v.nameInput.View() + "\n\n" +
			keyHelp("↵", confirmHint, "esc", "cancel")
	}
	if v.browserManual {
		actionsBody = mutedStyle.Render("Browser command (runs on the local machine):") + "\n\n" +
			v.browserInput.View() + "\n\n" +
			keyHelp("↵", "save", "esc", "cancel")
	}
	right := paneTitle("DETAILS", v.focus == focusActions) + "\n\n" +
		info + "\n\n" + rule(rightW) + "\n" +
		paneTitle(fmt.Sprintf("ACTIONS (%d)", len(acts)), v.focus == focusActions && !v.suspended) + "\n\n" + actionsBody
	switch {
	case v.replacePicking:
		right = paneTitle("REPLACE WITH", true) + "\n\n" +
			mutedStyle.Render(fmt.Sprintf("Repoint %d mapped dir(s) at:", len(v.linkedDirs))) + "\n\n" +
			v.replacePick.view(rightW)
	case v.confirming && len(v.linkedDirs) > 0:
		right = paneTitle("CONFIRM", true) + "\n\n" +
			mutedStyle.Render("Mapped in:\n"+formatDirList(v.linkedDirs)) + "\n\n" +
			v.confirm.view(rightW)
	case v.confirming:
		right = paneTitle("CONFIRM", true) + "\n\n" +
			mutedStyle.Render("Removes its conf, token dir,\nand this dir's "+v.prov.Scheme().Pointer+".") + "\n\n" +
			v.confirm.view(rightW)
	}

	contentW, _, _ := paneDims(v.width)
	help := keyHelpFit(contentW,
		[]string{"↑↓", "select", "↵", "open/run", "n", "new profile", "esc", "back"},
		[]string{"q", "quit", "→", "details", "⇥", "tab", "o", "options", "d", "change directory"})
	switch {
	case v.replacePicking:
		help = keyHelp("↑↓", "choose", "↵", "confirm", "esc", "cancel")
	case v.confirming && len(v.linkedDirs) > 0:
		help = keyHelp("↑↓", "choose", "↵", "confirm", "y", "unmap+delete", "n/esc", "cancel")
	case v.confirming:
		help = keyHelp("↑↓", "choose", "↵", "confirm", "y", "yes", "n/esc", "cancel")
	}
	view := renderPaneFrame(v.width, v.height, v.identityStrip(), left, right, scopeLegend(leftW), v.status, help)
	if v.browserPick != nil {
		return overlayCenter(view, v.browserPick.view(), v.width)
	}
	return view
}

// clampAction keeps the action cursor inside the selection's enabled set.
func (v *providerTabView) clampAction() {
	if n := len(v.enabledActions()); v.actionCur >= n && n > 0 {
		v.actionCur = n - 1
	}
}
