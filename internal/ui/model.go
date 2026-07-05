package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/browserpick"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// focus identifies which pane receives navigation keys.
const (
	focusProfiles = iota
	focusActions
)

// homeActions is the action radio group; keys double as hotkey accelerators.
var homeActions = []radioOption{
	{label: "Sign in", key: "l", hint: "session only — no pin"},
	{label: "Use here", key: "u", hint: "pin only — no login"},
	{label: "Capture session", key: "c", hint: "save current login"},
	{label: "New profile", key: "i", hint: "sign in + pin here"},
	{label: "Edit…", key: "x", hint: "open .conf in $EDITOR"},
	{label: "Rename…", key: "n", hint: "change profile name"},
	{label: "Browser profile", key: "b", hint: "map to a local browser profile"},
	{label: "Remove…", key: "delete", hint: "delete profile"},
}

// accountShowFn is overridable in tests; it reports the az identity for a
// specific profile config dir.
var accountShowFn = azure.AccountShowIn

// profileDelegate hand-renders profile rows so the leading scope icon keeps
// its own colour without fighting the row style (a styled glyph inside the
// stock delegate's Render would reset the selection styling mid-line): each
// segment — selection bar, icon, name, detail — is styled independently and
// composed. One blank line between rows keeps each two-line profile distinct.
type profileDelegate struct {
	mode selMode // how this pane's selection renders in the focus hierarchy
}

func (profileDelegate) Height() int                         { return 2 }
func (profileDelegate) Spacing() int                        { return 1 }
func (profileDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d profileDelegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	p, ok := li.(item)
	if !ok {
		return
	}
	textW := m.Width() - 5 // selection bar/pad (2) + icon slot (3)
	if textW < 1 {
		textW = 1
	}
	selected := index == m.Index()
	nameStyle := lipgloss.NewStyle().Foreground(white)
	detailStyle := mutedStyle
	switch {
	case selected && d.mode == selActive:
		nameStyle = selBlockActive
		detailStyle = lipgloss.NewStyle().Foreground(azureSky)
	case selected && d.mode == selParent:
		nameStyle = selBlockParent
		detailStyle = lipgloss.NewStyle().Foreground(gray)
	}
	name := p.name
	if p.label != "" && p.label != p.name {
		// Renamed rows keep their italic through selection.
		name = p.label
		nameStyle = nameStyle.Italic(true)
		if !selected {
			nameStyle = nameStyle.Foreground(whiteDim)
		}
	}
	if p.emph {
		nameStyle = nameStyle.Bold(true)
	}
	fmt.Fprintf(w, "%s%s\n   %s",
		scopeSlot(p.scope), nameStyle.Render(truncateLine(name, textW)),
		detailStyle.Render(truncateLine(p.tenant, textW)))
}

// item is one profile row. name is the immutable slug (identity, used for all
// operations and the CLI); label is the optional display name. When a label is
// set, the slug is shown alongside the tenant so it stays discoverable.
type item struct {
	name, label, tenant string
	scope               string
	emph                bool
}

func (i item) FilterValue() string { return i.name + " " + i.label }

// Model is the root TUI model.
type Model struct {
	list          list.Model
	statuses      map[string]provider.Status
	profs         []profile.Listed
	ambIdent      string
	dirProfile    string
	actions       radio
	confirm       radio
	spin          spinner.Model
	rename        textinput.Model
	pwd           string
	width, height int
	status        string
	signedIn      string
	ambientWho    string
	focus         int
	busy          bool
	confirming    bool
	renaming      bool
	showHelp      bool
	drift         bool
	ambientEmpty  bool
	pendingDelete string
	renameOld     string
	suspended     bool
	touched       bool
	dirScope      string
	creating      bool
	create        textinput.Model

	browserFor    string // profile a browser mapping is being chosen for
	browserPick   *browserPicker
	browserManual bool
	browserInput  textinput.Model
}

// NewModel builds the home model from the profiles on disk.
func NewModel() Model {
	pwd, _ := os.Getwd()
	var items []list.Item
	profs, _ := profile.List(config.ProfilesDir())
	statusMap := make(map[string]provider.Status, len(profs))
	statuses := make([]provider.Status, 0, len(profs))
	for _, p := range profs {
		if st, err := azure.NewProvider().Status(p.Name, config.ProfilesDir()); err == nil {
			statusMap[p.Name] = st
			statuses = append(statuses, st)
		}
	}
	active, ambIdent := "", ""
	if amb, err := azure.NewProvider().Ambient(); err == nil && amb.Identity != "" {
		ambIdent = amb.Identity
		active = provider.MatchProfile(statuses, amb.Identity)
	}
	dirProfile, dirScope := "", ""
	if name, err := profile.Resolve("", pwd); err == nil && name != "" {
		dirProfile = name
		dirScope = ScopeAncestor
		if d, ok := profile.LocateAzprofile(pwd); ok && d == pwd {
			dirScope = ScopeCwd
		}
	}
	mapped := map[string]bool{}
	for _, mp := range azure.NewProvider().Scheme().ReadMappings(config.ProfilesDir()) {
		mapped[mp.Profile] = true
	}
	// The most-active profile — the one that would be used right now — renders
	// bold: the dir-pinned one when present, else the global default.
	emph := dirProfile
	if emph == "" {
		emph = active
	}
	for _, p := range profs {
		scope := ""
		switch {
		case p.Name == dirProfile:
			scope = dirScope
		case p.Name == active:
			scope = scopeGlobal
		case mapped[p.Name]:
			scope = scopeElsewhere
		}
		items = append(items, item{name: p.Name, label: p.Label, tenant: p.Detail, scope: scope, emph: p.Name == emph})
	}
	l := list.New(items, profileDelegate{mode: selActive}, 0, 0)
	l.Title = "Profiles"
	l.SetShowHelp(false)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = dotStyle
	m := Model{list: l, spin: sp, pwd: pwd, actions: newRadio(homeActions),
		statuses: statusMap, profs: profs, ambIdent: ambIdent, dirProfile: dirProfile, dirScope: dirScope}
	m.rebuildActions()
	m.applyFocus()
	return m
}

// rebuildActions filters the action set for the current selection — actions
// that cannot apply are hidden (Use here when the selected profile already
// pins this directory) — preserving the cursor by key where possible.
func (m *Model) rebuildActions() {
	sel, _ := m.list.SelectedItem().(item)
	cur := ""
	if len(m.actions.options) > 0 && m.actions.cursor < len(m.actions.options) {
		cur = m.actions.options[m.actions.cursor].key
	}
	opts := make([]radioOption, 0, len(homeActions))
	for _, o := range homeActions {
		if len(m.list.Items()) == 0 && o.key != "i" && o.key != "c" {
			// Nothing to select: only the bootstrap actions apply
			// (New profile, and Capture for an already-signed-in az).
			continue
		}
		if o.key == "u" && sel.name != "" && sel.name == m.dirProfile && m.dirScope == ScopeCwd {
			continue
		}
		if o.key == "l" && sel.name != "" && sessionLive(m.statuses[sel.name]) {
			// Sign in is the recovery verb; a live session has nothing to recover.
			continue
		}
		opts = append(opts, o)
	}
	m.actions.options = opts
	if !m.actions.selectByKey(cur) {
		m.actions.cursor = 0
	}
	m.applyFocus()
}

// Init implements tea.Model — kicks off the identity lookup for this dir.
func (m Model) Init() tea.Cmd { return m.identityCmd() }

// identityMsg carries the signed-in identity of this dir's profile session
// ("" when that profile has no live session), the ambient session's identity,
// and whether they differ with no .envrc pinning them together (drift). Both
// identities are tenant-qualified, so a B2B guest signed into two tenants
// with one UPN still reads as drift.
type identityMsg struct {
	who          string
	ambientWho   string
	drift        bool
	ambientEmpty bool
}

// identityCmd reads the account from the resolved profile's token dir, so the
// strip reflects who you'd be in this dir — not the ambient ~/.azure session.
// When a profile is resolved but the ambient `az` shows a different identity
// and no .envrc pins it, it flags drift so the UI can offer to write one.
func (m Model) identityCmd() tea.Cmd {
	pwd := m.pwd
	return func() tea.Msg {
		name, rErr := profile.Resolve("", pwd)
		dir := ""
		if rErr == nil {
			dir = filepath.Join(config.ProfilesDir(), name)
		}
		who := identityOf(accountShowFn(dir))
		msg := identityMsg{who: who}
		envrcDir := pwd
		if d, ok := profile.LocateAzprofile(pwd); ok {
			envrcDir = d
		}
		if rErr == nil && who != "" && !profile.HasEnvrc(envrcDir) {
			msg.ambientWho = identityOf(accountShowFn(""))
			msg.drift = msg.ambientWho != who
			msg.ambientEmpty = msg.ambientWho == ""
		}
		return msg
	}
}

// identityOf extracts the tenant-qualified identity from `az account show`
// output — the same composition the disk-only readers use, so comparisons
// stay tenant-aware (B2B guests share a UPN across tenants).
func identityOf(b []byte, err error) string {
	if err != nil {
		return ""
	}
	var a profile.AccountJSON
	if json.Unmarshal(b, &a) != nil {
		return ""
	}
	return azure.QualifiedIdentity(a.User.Name, a.TenantDefaultDomain, a.TenantID)
}

// paneDims computes the shared content and two-pane column widths for a given
// terminal width. It is the single source of truth for the canonical layout,
// used by the Azure Model, the provider tabs, and the frame renderer so every
// tab lines its panes up identically. contentW is the room inside the frame
// (border + padding), leftW/rightW the two column widths flanking the seam.
func paneDims(width int) (contentW, leftW, rightW int) {
	contentW = width - 4
	if contentW < 1 {
		contentW = 1
	}
	leftW = contentW * 40 / 100
	if leftW < 18 {
		leftW = 18
	}
	rightW = contentW - leftW - 3
	if rightW < 10 {
		rightW = 10
	}
	return
}

// dims computes the shared content width and pane sizes so layout() and View()
// stay in lockstep. contentW tracks the real terminal width (the banner now
// lives in the tab container, so this view no longer floors to the art width);
// the container truncates any residual overflow.
func (m Model) dims() (contentW, leftW, rightW, listH int) {
	contentW, leftW, rightW = paneDims(m.width)
	// chrome: 2 rules + identity + status + help + frame + the 3-row legend.
	listH = m.height - 11
	if listH < 3 {
		listH = 3
	}
	return
}

// layout recomputes child sizes and refreshes focus styling.
func (m *Model) layout() {
	_, leftW, _, listH := m.dims()
	m.list.SetSize(leftW, listH)
	m.applyFocus()
}

// applyFocus propagates the focus hierarchy into the child styles: the radio
// brightens only when the detail pane is focused, the list delegate only when
// the profiles pane is — and neither while the tab bar holds focus.
func (m *Model) applyFocus() {
	m.actions.focused = m.focus == focusActions && !m.suspended
	mode := selNone
	switch {
	case m.suspended:
		// The tab bar holds focus: no selection below it.
	case m.focus == focusProfiles:
		mode = selActive
	case m.focus == focusActions:
		mode = selParent
	}
	m.list.SetDelegate(profileDelegate{mode: mode})
}

// refresh rebuilds the profile list from disk, preserving view state.
func (m Model) refresh() Model {
	nm := NewModel()
	nm.width, nm.height = m.width, m.height
	nm.focus = m.focus
	nm.actions.cursor = m.actions.cursor
	nm.signedIn = m.signedIn
	nm.drift = m.drift
	nm.ambientEmpty = m.ambientEmpty
	nm.status = m.status
	nm.layout()
	return nm
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil
	case identityMsg:
		m.signedIn = msg.who
		m.ambientWho = msg.ambientWho
		m.drift = msg.drift
		m.ambientEmpty = msg.ambientEmpty
		return m, nil
	case switchTabMsg:
		// The dashboard jumped to this tab for a specific profile; move the cursor
		// onto it so it's pre-selected. No-op when the profile isn't listed here.
		for i, it := range m.list.Items() {
			if p, ok := it.(item); ok && p.name == msg.profile {
				m.list.Select(i)
				break
			}
		}
		return m, nil
	case barFocusMsg:
		m.suspended = msg.focused
		if !msg.focused {
			// Navigating down from the tab bar counts as entering the pane.
			m.touched = true
		}
		m.applyFocus()
		return m, nil
	case cwdChangedMsg:
		// The header shows the directory; no bottom-bar echo needed.
		nm := m.refresh()
		return nm, nm.identityCmd()
	case opDoneMsg:
		nm := m.refresh()
		nm.busy = false
		if msg.err != nil {
			nm.status = failureStyle.Render("✗ " + msg.err.Error())
		} else {
			nm.status = successStyle.Render("✓ " + msg.msg)
		}
		return nm, nm.identityCmd()
	case browserProfilesMsg:
		if msg.forProfile != m.browserFor || m.browserFor == "" {
			return m, nil
		}
		if msg.err != nil || len(msg.profiles) == 0 {
			ti := textinput.New()
			ti.Placeholder = "e.g. microsoft-edge --profile-directory=\"Profile 2\""
			ti.Focus()
			m.browserInput = ti
			m.browserManual = true
			m.status = mutedStyle.Render("discovery unavailable — enter a command")
			return m, nil
		}
		ident := m.statuses[m.browserFor].Identity
		pk := newBrowserPicker(msg.profiles, ident)
		m.browserPick = &pk
		return m, nil
	case spinner.TickMsg:
		var c tea.Cmd
		m.spin, c = m.spin.Update(msg)
		return m, c
	case tea.KeyMsg:
		if m.busy {
			return m, nil
		}
		if m.confirming {
			return m.updateConfirm(msg)
		}
		if m.renaming {
			return m.updateRename(msg)
		}
		if m.creating {
			return m.updateCreate(msg)
		}
		if m.browserPick != nil {
			return m.updateBrowserPick(msg)
		}
		if m.browserManual {
			return m.updateBrowserManual(msg)
		}
		return m.updateKey(msg)
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// capturesInput reports whether the rename text input is active, so the tab
// container forwards arrow/bracket keys here instead of switching tabs.
func (m Model) capturesInput() bool {
	return m.renaming || m.creating || m.browserManual || m.browserPick != nil
}

// updateKey handles the home-screen keymap.
func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k := msg.String(); k != "q" && k != "ctrl+c" {
		// Any navigation marks the pane as visited (bold titles from here on).
		m.touched = true
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "f5":
		return m.refresh(), nil
	case "right":
		m.focus = focusActions
		m.applyFocus()
		return m, nil
	case "esc", "left":
		if m.focus == focusActions {
			m.focus = focusProfiles
			m.applyFocus()
			return m, nil
		}
	case "enter":
		// Selecting a profile opens the action pane; enter there runs the action.
		if m.focus == focusProfiles {
			m.focus = focusActions
			m.applyFocus()
			return m, nil
		}
		return m.dispatch(m.actions.selected().key)
	case "l", "u", "c", "i", "x", "n", "b", "delete":
		if !m.actions.selectByKey(msg.String()) {
			// The action is hidden for this selection (e.g. Use here on the
			// already-pinned profile).
			return m, nil
		}
		return m.dispatch(msg.String())
	case "e":
		return m.dispatch("e")
	case "up", "k":
		if m.focus == focusActions {
			m.actions.up()
			return m, nil
		}
		if m.list.Index() == 0 {
			// Already at the top of the list: hand focus to the tab bar.
			return m, func() tea.Msg { return focusTabsMsg{} }
		}
		defer func() { m.rebuildActions() }()
	case "down", "j":
		if m.focus == focusActions {
			m.actions.down()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m.rebuildActions()
	return m, cmd
}

// updateConfirm handles the remove confirmation sub-state.
func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "n", "q":
		m.confirming = false
		m.pendingDelete = ""
		return m, nil
	case "y":
		return m.doRemove()
	case "up", "k", "left":
		m.confirm.up()
		return m, nil
	case "down", "j", "right":
		m.confirm.down()
		return m, nil
	case "enter":
		if m.confirm.cursor == 1 {
			return m.doRemove()
		}
		m.confirming = false
		m.pendingDelete = ""
		return m, nil
	}
	return m, nil
}

// updateRename handles the rename text-input sub-state.
// updateCreate handles the new-profile name prompt: enter execs
// `login <name> --yes`, whose create path signs in and pins this directory.
func (m Model) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.creating = false
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.create.Value())
		if name == "" {
			name = m.create.Placeholder
		}
		name = profile.SanitizeName(name)
		if name == "" {
			return m, nil
		}
		m.creating = false
		m.busy = true
		m.status = ""
		return m, runHandoff([]string{"login", name, "--yes"})
	}
	var cmd tea.Cmd
	m.create, cmd = m.create.Update(msg)
	return m, cmd
}

func (m Model) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.renaming = false
		m.renameOld = ""
		return m, nil
	case "enter":
		label := strings.TrimSpace(m.rename.Value())
		slug := m.renameOld
		m.renaming = false
		m.renameOld = ""
		// a label equal to the slug is stored as empty (display falls back to slug).
		if label == slug {
			label = ""
		}
		m.busy = true
		m.status = ""
		return m, tea.Batch(m.spin.Tick, runRelabel(slug, label))
	}
	var cmd tea.Cmd
	m.rename, cmd = m.rename.Update(msg)
	return m, cmd
}

// updateBrowserPick routes keys to the browser-profile overlay picker.
func (m Model) updateBrowserPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	np, picked, closed := m.browserPick.update(msg)
	m.browserPick = &np
	if closed {
		m.browserPick = nil
		if picked != nil {
			m.applyBrowserMapping(picked.Command(), picked.Label())
		} else {
			m.status = ""
		}
		m.browserFor = ""
	}
	return m, nil
}

// updateBrowserManual handles the manual browser-command fallback prompt.
func (m Model) updateBrowserManual(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.browserManual = false
		m.browserFor = ""
		m.status = ""
	case "enter":
		if c := strings.TrimSpace(m.browserInput.Value()); c != "" {
			m.browserManual = false
			m.applyBrowserMapping(c, "")
			m.browserFor = ""
		}
	default:
		var cmd tea.Cmd
		m.browserInput, cmd = m.browserInput.Update(msg)
		_ = cmd
	}
	return m, nil
}

// applyBrowserMapping writes the browser cmd/label keys for browserFor.
func (m *Model) applyBrowserMapping(cmdVal, labelVal string) {
	cmdKey, labelKey := browserpick.Keys("azure")
	sch := azure.NewProvider().Scheme()
	dir := config.ProfilesDir()
	if err := sch.SetKey(m.browserFor, dir, cmdKey, cmdVal); err != nil {
		m.status = failureStyle.Render(err.Error())
		return
	}
	if err := sch.SetKey(m.browserFor, dir, labelKey, labelVal); err != nil {
		m.status = failureStyle.Render(err.Error())
		return
	}
	disp := labelVal
	if disp == "" {
		disp = cmdVal
	}
	m.status = successStyle.Render(fmt.Sprintf("%q opens with %s", m.browserFor, disp))
}

func (m Model) doRemove() (tea.Model, tea.Cmd) {
	name := m.pendingDelete
	m.confirming = false
	m.pendingDelete = ""
	m.busy = true
	m.status = ""
	return m, tea.Batch(m.spin.Tick, runDelete(name))
}

// dispatch runs the action identified by key against the selected profile.
func (m Model) dispatch(key string) (tea.Model, tea.Cmd) {
	sel, _ := m.list.SelectedItem().(item)
	switch key {
	case "u":
		if sel.name == "" {
			m.status = failureStyle.Render("✗ select a profile to link this dir to")
			return m, nil
		}
		m.busy = true
		m.status = ""
		return m, tea.Batch(m.spin.Tick, runUse(sel.name))
	case "delete":
		if sel.name == "" {
			m.status = failureStyle.Render("✗ select a profile to remove")
			return m, nil
		}
		m.confirming = true
		m.pendingDelete = sel.name
		m.confirm = newRadio([]radioOption{
			{label: "No, keep it"},
			{label: "Yes, remove " + sel.name},
		})
		m.confirm.focused = true
		return m, nil
	case "l":
		m.busy = true
		m.status = ""
		return m, runHandoff(handoffArgs("l", sel.name))
	case "i":
		// New profile needs a name — the bare login picker only selects
		// existing profiles and would never hit the pin-on-create path.
		pwd, _ := os.Getwd()
		ti := textinput.New()
		ti.Placeholder = profile.DefaultName("", pwd)
		ti.Width = 28
		cmd := ti.Focus()
		m.create = ti
		m.creating = true
		m.status = ""
		return m, cmd
	case "x":
		if sel.name == "" {
			m.status = failureStyle.Render("✗ select a profile to edit")
			return m, nil
		}
		m.busy = true
		m.status = ""
		return m, runEdit(sel.name)
	case "n":
		if sel.name == "" {
			m.status = failureStyle.Render("✗ select a profile to rename")
			return m, nil
		}
		ti := textinput.New()
		// Seed with the raw display name — Title() carries the ●/○ status dot
		// and the alias marker, which must not leak into the new label.
		disp := sel.label
		if disp == "" {
			disp = sel.name
		}
		ti.SetValue(disp)
		ti.CursorEnd()
		ti.Width = 28
		cmd := ti.Focus()
		m.rename = ti
		m.renaming = true
		m.renameOld = sel.name
		m.status = ""
		return m, cmd
	case "c":
		m.busy = true
		m.status = ""
		return m, runHandoff(handoffArgs("c", ""))
	case "b":
		if sel.name == "" {
			m.status = failureStyle.Render("✗ select a profile to map a browser to")
			return m, nil
		}
		m.browserFor = sel.name
		m.status = mutedStyle.Render("looking for browser profiles on the local machine…")
		return m, discoverBrowsersCmd(sel.name)
	case "e":
		if _, err := profile.Resolve("", m.pwd); err != nil {
			m.status = failureStyle.Render("✗ no profile here to pin")
			return m, nil
		}
		m.busy = true
		m.status = ""
		return m, tea.Batch(m.spin.Tick, runWriteEnvrc())
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	_, _, rightW, _ := m.dims()

	// The bubbles list emits its own leading blank line, so no extra spacer is
	// needed here — that keeps the first profile row aligned with the first
	// action row (which sits below "ACTION" + one blank line).
	_, lw, _, _ := m.dims()
	left := lipgloss.JoinVertical(lipgloss.Left,
		paneTitle(fmt.Sprintf("PROFILES (%d)", len(m.list.Items())), m.focus == focusProfiles && !m.suspended && m.touched),
		m.list.View(),
	)

	statusLine := m.status
	if m.busy {
		statusLine = m.spin.View() + mutedStyle.Render(" working…")
	}
	view := renderPaneFrame(m.width, m.height, m.identityStrip(), left, m.rightPane(rightW), scopeLegend(lw), statusLine, m.helpBar())
	if m.browserPick != nil {
		return overlayCenter(view, m.browserPick.view(), m.width)
	}
	return view
}

// renderPaneFrame draws the canonical azrl layout that every tab shares so they
// look identical: a header rule, a centered identity strip, a rule, a two-pane
// body (left/right already rendered), a rule, a centered status line, and a
// centered footer — filled to the full terminal width and height and wrapped in
// the frame. All content lines are padded to the content width so the frame
// spans the terminal edge-to-edge, and truncated so no line ever overflows it.
func renderPaneFrame(width, height int, identity, left, right, leftFoot, status, footer string) string {
	contentW, leftW, _ := paneDims(width)
	center := func(s string) string { return lipgloss.PlaceHorizontal(contentW, lipgloss.Center, s) }

	// No rule above the header — the frame's top border already bounds it.
	head := lipgloss.JoinVertical(lipgloss.Left, center(identity), rule(contentW))
	foot := lipgloss.JoinVertical(lipgloss.Left, rule(contentW), center(status), center(footer))

	// Vertical fill: grow the body so the frame bottom sits near the terminal
	// bottom (frame border = 2 rows) instead of a short box with dead space below.
	bodyH := height - 2 - lipgloss.Height(head) - lipgloss.Height(foot)
	if bodyH < 1 {
		bodyH = 1
	}
	// leftFoot (the icon legend) anchors to the bottom of the left column,
	// padded away from the rows above; at tiny heights it follows them directly.
	if leftFoot != "" {
		ll := strings.Split(left, "\n")
		lf := strings.Split(leftFoot, "\n")
		for pad := bodyH - len(ll) - len(lf); pad > 0; pad-- {
			ll = append(ll, "")
		}
		left = strings.Join(append(ll, lf...), "\n")
	}
	body := joinColumns(left, right, leftW, contentW, bodyH)

	content := lipgloss.JoinVertical(lipgloss.Left, head, body, foot)
	// Normalize every line to exactly contentW: truncate overflow (invariant) and
	// pad short lines so the frame border reaches the terminal's right edge.
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = padTo(truncateLine(l, contentW), contentW)
	}
	return frameStyle.Render(strings.Join(lines, "\n"))
}

// renderProfilePane hand-renders a PROFILES(n) pane for a slice of profiles,
// mirroring the Azure list delegate (selection bar on the focused row, muted
// details) so the provider tabs match the Azure Model without a bubbles list.
// Rows lead with their active-identity icon (scopeSlot: ● green cwd pin, ●
// orange parent pin, 🌐 global default); renamed profiles render their label
// in the renamedStyle accent instead of a footnote legend. Segments are
// styled independently so the icon keeps its own colour on selected rows.
func renderProfilePane(profiles []profile.Listed, cursor int, mode selMode, touched bool, leftW int, scopes map[string]string) string {
	var b strings.Builder
	b.WriteString(paneTitle(fmt.Sprintf("PROFILES (%d)", len(profiles)), mode == selActive && touched))
	b.WriteString("\n\n")
	if len(profiles) == 0 {
		b.WriteString(mutedStyle.Render("  (none yet — ") + keycap("a") + mutedStyle.Render(" creates one)"))
		return b.String()
	}
	textW := leftW - 5 // selection bar/pad (2) + icon slot (3)
	if textW < 1 {
		textW = 1
	}
	// The most-active profile — the one that would be used right now — renders
	// bold: the dir-pinned row when one exists, else the global default.
	emph := ""
	for _, p := range profiles {
		switch scopes[p.Name] {
		case ScopeCwd, ScopeAncestor:
			emph = p.Name
		case scopeGlobal:
			if emph == "" {
				emph = p.Name
			}
		}
	}
	for i, p := range profiles {
		if i > 0 {
			// One blank line between rows keeps each two-line profile distinct,
			// matching the Azure list delegate's spacing.
			b.WriteString("\n")
		}
		selected := i == cursor
		nameStyle := lipgloss.NewStyle().Foreground(white)
		detailStyle := mutedStyle
		switch {
		case selected && mode == selActive:
			// The shared selection block: bright in the focused container.
			nameStyle = selBlockActive
			detailStyle = lipgloss.NewStyle().Foreground(azureSky)
		case selected && mode == selParent:
			// A child holds focus: this level's selection dims as the trail.
			nameStyle = selBlockParent
			detailStyle = lipgloss.NewStyle().Foreground(gray)
		}
		if p.Label != "" && p.Label != p.Name {
			// Renamed rows keep their italic through selection.
			nameStyle = nameStyle.Italic(true)
			if !selected {
				nameStyle = nameStyle.Foreground(whiteDim)
			}
		}
		if p.Name == emph {
			nameStyle = nameStyle.Bold(true)
		}
		b.WriteString(scopeSlot(scopes[p.Name]) + nameStyle.Render(truncateLine(p.Display(), textW)) + "\n")
		b.WriteString("   " + detailStyle.Render(truncateLine(p.Detail, textW)) + "\n")
	}
	return b.String()
}

// joinColumns zips two blocks into a two-pane body of exactly totalW columns,
// with a vertical seam between them; both columns are padded so the seam runs
// full height and the right edge lines up with the rules and frame. The body is
// grown to at least minH rows so it fills the available vertical space.
func joinColumns(left, right string, leftW, totalW, minH int) string {
	seam := dividerStyle.Render("│")
	rightW := totalW - leftW - 3
	if rightW < 0 {
		rightW = 0
	}
	ll := strings.Split(left, "\n")
	rl := strings.Split(right, "\n")
	n := len(ll)
	if len(rl) > n {
		n = len(rl)
	}
	if minH > n {
		n = minH
	}
	rows := make([]string, n)
	for i := 0; i < n; i++ {
		var l, r string
		if i < len(ll) {
			l = ll[i]
		}
		if i < len(rl) {
			r = rl[i]
		}
		rows[i] = padTo(l, leftW) + " " + seam + " " + padTo(r, rightW)
	}
	return strings.Join(rows, "\n")
}

// padTo right-pads s with spaces to a visible width of w (ANSI-aware).
func padTo(s string, w int) string {
	if gap := w - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// rightPane renders either the action radio group or the remove confirmation.
func (m Model) rightPane(w int) string {
	if m.confirming {
		prompt := paneTitle("CONFIRM", true) + "\n\n" +
			mutedStyle.Render("Removes its conf, token dir,\nand this dir's .azprofile.") + "\n\n"
		return prompt + m.confirm.view(w)
	}
	if m.renaming {
		return paneTitle("RENAME", true) + "\n\n" +
			mutedStyle.Render("New name for "+m.renameOld+":") + "\n\n" +
			m.rename.View() + "\n\n" +
			keyHelp("↵", "rename", "esc", "cancel")
	}
	if m.creating {
		return paneTitle("NEW PROFILE", true) + "\n\n" +
			mutedStyle.Render("Name for the new profile:") + "\n\n" +
			m.create.View() + "\n\n" +
			keyHelp("↵", "create + sign in + pin", "esc", "cancel")
	}
	if m.browserManual {
		return paneTitle("BROWSER PROFILE", true) + "\n\n" +
			mutedStyle.Render("Browser command (runs on the local machine):") + "\n\n" +
			m.browserInput.View() + "\n\n" +
			keyHelp("↵", "save", "esc", "cancel")
	}
	info := mutedStyle.Render("(no profile selected)")
	if it, ok := m.list.SelectedItem().(item); ok {
		pr := profile.Listed{Name: it.name, Label: it.label, Detail: it.tenant}
		st := m.statuses[it.name]
		note := ""
		if st.Drifted {
			note = "shell uses " + orNoSession(m.ambIdent)
		}
		cmdKey, labelKey := browserpick.Keys("azure")
		sch := azure.NewProvider().Scheme()
		browser := sch.GetKey(it.name, config.ProfilesDir(), labelKey)
		if browser == "" {
			browser = sch.GetKey(it.name, config.ProfilesDir(), cmdKey)
		}
		info = profileInfoBlock(pr, st, browser, note, w)
	}
	actionsBody := m.actions.view(w)
	return paneTitle("DETAILS", m.focus == focusActions) + "\n\n" +
		info + "\n\n" + rule(w) + "\n" +
		paneTitle(fmt.Sprintf("ACTIONS (%d)", len(m.actions.options)), m.focus == focusActions && !m.suspended) + "\n\n" + actionsBody
}

// paneTitle renders a pane header: bold for the focused pane (the selection
// block below carries the strong cue), muted otherwise.
func paneTitle(s string, active bool) string {
	if active {
		return paneTitleStyle.Render(s)
	}
	return mutedStyle.Render(s)
}

func rule(w int) string {
	if w < 1 {
		w = 1
	}
	return dividerStyle.Render(strings.Repeat("─", w))
}

// identityStrip is the standard provider header (icon · dir · effective
// identity), plus Azure's drift warning offering to pin the shell.
func (m Model) identityStrip() string {
	dirIdentity := m.statuses[m.dirProfile].Identity
	if m.signedIn != "" {
		// The async az account show is the freshest source for the pinned dir.
		dirIdentity = m.signedIn
	}
	contentW, _, _ := paneDims(m.width)
	strip := headerStrip(contentW, providerIcon("azure"), "Azure", m.pwd,
		effectiveIdentity(m.dirProfile, dirIdentity, m.ambIdent))
	if m.drift {
		what := "is " + m.ambientWho
		if m.ambientEmpty {
			what = "has no active session"
		}
		warning := failureStyle.Render("⚠ shell az "+what+" — this dir expects "+m.signedIn) +
			mutedStyle.Render(" · ") + keycap("e") + mutedStyle.Render(" writes .envrc")
		strip += "\n" + ansi.Wordwrap(warning, contentW, "")
	}
	return strip
}

// helpBar lists only the keys that are actually wired.
func (m Model) helpBar() string {
	if m.confirming {
		return keyHelp("↑↓", "choose", "↵", "confirm", "y", "yes", "n/esc", "cancel")
	}
	if m.renaming {
		return mutedStyle.Render("type new name · ") + keyHelp("↵", "rename", "esc", "cancel")
	}
	if m.creating {
		return mutedStyle.Render("type a profile name · ") + keyHelp("↵", "create + sign in + pin", "esc", "cancel")
	}
	if m.browserManual {
		return mutedStyle.Render("type a browser command · ") + keyHelp("↵", "save", "esc", "cancel")
	}
	if m.showHelp {
		lines := []string{
			keycap("l") + " sign in   " + keycap("u") + " use here   " + keycap("c") + " capture   " + keycap("e") + " write .envrc",
			keycap("i") + " new profile   " + keycap("x") + " edit   " + keycap("n") + " rename   " + keycap("b") + " browser   " + keycap("delete") + " remove",
			keyHelp("↑↓", "select", "↵", "open/run", "esc", "back", "f5", "refresh", "?", "less", "q", "quit"),
		}
		return strings.Join(lines, "\n")
	}
	contentW, _, _ := paneDims(m.width)
	return keyHelpFit(contentW,
		[]string{"↑↓", "select", "↵", "open/run", "esc", "back"},
		[]string{"q", "quit", "?", "help", "→", "details", "⇥", "tab", "d", "dir", "o", "options", "f5", "refresh"})
}
