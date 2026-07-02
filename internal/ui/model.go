package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
)

// focus identifies which pane receives navigation keys.
const (
	focusProfiles = iota
	focusActions
)

// homeActions is the action radio group; keys double as hotkey accelerators.
var homeActions = []radioOption{
	{label: "Sign in", key: "l", hint: "bridge + az login"},
	{label: "Use here", key: "u", hint: "link this dir"},
	{label: "Capture session", key: "c", hint: "save current login"},
	{label: "New profile", key: "i", hint: "init + sign in"},
	{label: "Edit…", key: "x", hint: "open .conf in $EDITOR"},
	{label: "Rename…", key: "n", hint: "change profile name"},
	{label: "Remove…", key: "delete", hint: "delete profile"},
}

// accountShowFn is overridable in tests; it reports the az identity for a
// specific profile config dir.
var accountShowFn = azure.AccountShowIn

// selectionBar is the azure-palette selection marker shared by the profile list:
// a blue thick left bar with one column of padding. Both the bubbles list
// delegate and the provider tabs' hand-rendered profile pane reuse it so the
// selected-row look is identical everywhere.
var selectionBar = lipgloss.NewStyle().
	Border(lipgloss.ThickBorder(), false, false, false, true).
	BorderForeground(azureBlue).PaddingLeft(1)

// profileDelegate renders profile rows in the azure palette: a blue selection
// bar with a gold name, replacing the bubbles default magenta. One blank line
// between rows keeps each two-line profile visually distinct.
func profileDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.SetSpacing(1)
	d.Styles.SelectedTitle = selectionBar.Foreground(gold).Bold(true)
	d.Styles.SelectedDesc = selectionBar.Foreground(azureSky)
	d.Styles.NormalTitle = lipgloss.NewStyle().Foreground(white).PaddingLeft(2)
	d.Styles.NormalDesc = lipgloss.NewStyle().Foreground(gray).PaddingLeft(2)
	return d
}

// item is one profile row. name is the immutable slug (identity, used for all
// operations and the CLI); label is the optional display name. When a label is
// set, the slug is shown alongside the tenant so it stays discoverable.
type item struct{ name, label, tenant string }

// aliasMark flags a row whose display name is a custom label. The real slug is
// hidden (it lives in the .conf and the rename/edit dialogs), so the marker is
// what distinguishes a relabeled profile from a plain one at a glance. A "*"
// footnote in the help bar (see helpBar) explains it on screen.
const aliasMark = " *"

func (i item) Title() string {
	if i.label != "" && i.label != i.name {
		return i.label + aliasMark
	}
	return i.name
}

func (i item) Description() string {
	return i.tenant
}

func (i item) FilterValue() string { return i.name + " " + i.label }

// Model is the root TUI model.
type Model struct {
	list          list.Model
	actions       radio
	confirm       radio
	spin          spinner.Model
	rename        textinput.Model
	pwd           string
	width, height int
	status        string
	signedIn      string
	focus         int
	busy          bool
	confirming    bool
	renaming      bool
	showHelp      bool
	drift         bool
	ambientEmpty  bool
	pendingDelete string
	renameOld     string
}

// NewModel builds the home model from the profiles on disk.
func NewModel() Model {
	pwd, _ := os.Getwd()
	var items []list.Item
	profs, _ := profile.List(config.ProfilesDir())
	for _, p := range profs {
		items = append(items, item{name: p.Name, label: p.Label, tenant: p.Detail})
	}
	l := list.New(items, profileDelegate(), 0, 0)
	l.Title = "Profiles"
	l.SetShowHelp(false)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = dotStyle
	m := Model{list: l, spin: sp, pwd: pwd, actions: newRadio(homeActions)}
	m.applyFocus()
	return m
}

// Init implements tea.Model — kicks off the identity lookup for this dir.
func (m Model) Init() tea.Cmd { return m.identityCmd() }

// identityMsg carries the signed-in identity of this dir's profile session
// ("" when that profile has no live session) and whether the ambient `az`
// session differs from it with no .envrc pinning it (drift).
type identityMsg struct {
	who          string
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
		who := userOf(accountShowFn(dir))
		msg := identityMsg{who: who}
		envrcDir := pwd
		if d, ok := profile.LocateAzprofile(pwd); ok {
			envrcDir = d
		}
		if rErr == nil && who != "" && !profile.HasEnvrc(envrcDir) {
			ambient := userOf(accountShowFn(""))
			msg.drift = ambient != who
			msg.ambientEmpty = ambient == ""
		}
		return msg
	}
}

// userOf extracts the account's user name from `az account show` output.
func userOf(b []byte, err error) string {
	if err != nil {
		return ""
	}
	var a profile.AccountJSON
	if json.Unmarshal(b, &a) != nil {
		return ""
	}
	return a.User.Name
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
	// chrome: 3 rules + identity + status + help + frame.
	listH = m.height - 9
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

// applyFocus toggles the radio's focused styling to match the active pane.
func (m *Model) applyFocus() { m.actions.focused = m.focus == focusActions }

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
	case opDoneMsg:
		nm := m.refresh()
		nm.busy = false
		if msg.err != nil {
			nm.status = failureStyle.Render("✗ " + msg.err.Error())
		} else {
			nm.status = successStyle.Render("✓ " + msg.msg)
		}
		return nm, nm.identityCmd()
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
		return m.updateKey(msg)
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// capturesInput reports whether the rename text input is active, so the tab
// container forwards arrow/bracket keys here instead of switching tabs.
func (m Model) capturesInput() bool { return m.renaming }

// updateKey handles the home-screen keymap.
func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "f5":
		return m.refresh(), nil
	case "tab", "shift+tab":
		m.focus = focusActions - m.focus
		m.applyFocus()
		return m, nil
	case "esc":
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
	case "l", "u", "c", "i", "x", "n", "delete":
		m.actions.selectByKey(msg.String())
		return m.dispatch(msg.String())
	case "e":
		return m.dispatch("e")
	case "up", "k":
		if m.focus == focusActions {
			m.actions.up()
			return m, nil
		}
	case "down", "j":
		if m.focus == focusActions {
			m.actions.down()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
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
		m.busy = true
		m.status = ""
		return m, runHandoff(handoffArgs("i", ""))
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
		ti.SetValue(sel.Title())
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
	left := lipgloss.JoinVertical(lipgloss.Left,
		paneTitle(fmt.Sprintf("PROFILES (%d)", len(m.list.Items())), m.focus == focusProfiles),
		m.list.View(),
	)

	statusLine := m.status
	if m.busy {
		statusLine = m.spin.View() + mutedStyle.Render(" working…")
	}
	return renderPaneFrame(m.width, m.height, m.identityStrip(), left, m.rightPane(rightW), statusLine, m.helpBar())
}

// renderPaneFrame draws the canonical azrl layout that every tab shares so they
// look identical: a header rule, a centered identity strip, a rule, a two-pane
// body (left/right already rendered), a rule, a centered status line, and a
// centered footer — filled to the full terminal width and height and wrapped in
// the frame. All content lines are padded to the content width so the frame
// spans the terminal edge-to-edge, and truncated so no line ever overflows it.
func renderPaneFrame(width, height int, identity, left, right, status, footer string) string {
	contentW, leftW, _ := paneDims(width)
	center := func(s string) string { return lipgloss.PlaceHorizontal(contentW, lipgloss.Center, s) }

	head := lipgloss.JoinVertical(lipgloss.Left, rule(contentW), center(identity), rule(contentW))
	foot := lipgloss.JoinVertical(lipgloss.Left, rule(contentW), center(status), center(footer))

	// Vertical fill: grow the body so the frame bottom sits near the terminal
	// bottom (frame border = 2 rows) instead of a short box with dead space below.
	bodyH := height - 2 - lipgloss.Height(head) - lipgloss.Height(foot)
	if bodyH < 1 {
		bodyH = 1
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
// mirroring the Azure list delegate (selectionBar on the focused row, muted
// details) so the provider tabs match the Azure Model without a bubbles list.
func renderProfilePane(profiles []profile.Listed, cursor int, focused bool, leftW int) string {
	var b strings.Builder
	b.WriteString(paneTitle(fmt.Sprintf("PROFILES (%d)", len(profiles)), focused))
	b.WriteString("\n\n")
	if len(profiles) == 0 {
		b.WriteString(mutedStyle.Render("  (none yet — Sign in to add one)"))
		return b.String()
	}
	textW := leftW - 2
	if textW < 1 {
		textW = 1
	}
	for i, p := range profiles {
		if i > 0 {
			// One blank line between rows keeps each two-line profile distinct,
			// matching the Azure list delegate's spacing.
			b.WriteString("\n")
		}
		name := truncateLine(p.Display(), textW)
		detail := truncateLine(p.Detail, textW)
		switch {
		case i == cursor && focused:
			b.WriteString(selectionBar.Foreground(gold).Bold(true).Render(name) + "\n")
			b.WriteString(selectionBar.Foreground(azureSky).Render(detail) + "\n")
		case i == cursor:
			b.WriteString(selectionBar.Foreground(white).Render(name) + "\n")
			b.WriteString(selectionBar.Foreground(gray).Render(detail) + "\n")
		default:
			b.WriteString(lipgloss.NewStyle().Foreground(white).PaddingLeft(2).Render(name) + "\n")
			b.WriteString(mutedStyle.PaddingLeft(2).Render(detail) + "\n")
		}
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
			mutedStyle.Render("↵ rename · esc cancel")
	}
	return paneTitle("ACTION", m.focus == focusActions) + "\n\n" + m.actions.view(w)
}

// paneTitle renders a pane header; the focused pane's title becomes an
// inverted chip so it's obvious which pane keys act on.
func paneTitle(s string, active bool) string {
	if active {
		return paneFocusStyle.Render(s)
	}
	return mutedStyle.Render(s)
}

func rule(w int) string {
	if w < 1 {
		w = 1
	}
	return dividerStyle.Render(strings.Repeat("─", w))
}

// identityStrip shows this dir's profile and its signed-in identity, plus a
// drift warning offering to pin the shell with an .envrc.
func (m Model) identityStrip() string {
	left := accentStyle.Render("◆") + " " + contextLine(m.pwd)
	right := mutedStyle.Render("not signed in")
	if m.signedIn != "" {
		right = mutedStyle.Render("signed in ") + accentStyle.Render(m.signedIn) + successStyle.Render(" ✓")
	}
	strip := left + mutedStyle.Render("   ·   ") + right
	if m.drift {
		what := "uses a different account"
		if m.ambientEmpty {
			what = "has no active session"
		}
		strip += "\n" + failureStyle.Render("⚠ your shell's az "+what+" — press e to write .envrc")
	}
	return strip
}

// helpBar lists only the keys that are actually wired.
func (m Model) helpBar() string {
	if m.confirming {
		return mutedStyle.Render("↑↓ choose · ↵ confirm · y yes · n/esc cancel")
	}
	if m.renaming {
		return mutedStyle.Render("type new name · ↵ rename · esc cancel")
	}
	if m.showHelp {
		lines := []string{
			keycap("l") + " sign in   " + keycap("u") + " use here   " + keycap("c") + " capture   " + keycap("e") + " write .envrc",
			keycap("i") + " new profile   " + keycap("x") + " edit   " + keycap("n") + " rename   " + keycap("delete") + " remove",
			mutedStyle.Render("↑↓") + " select · " + mutedStyle.Render("↵") + " open/run · " + mutedStyle.Render("esc") + " back · " + keycap("f5") + " refresh · " + mutedStyle.Render("?") + " less · " + keycap("q") + " quit",
		}
		if m.hasAlias() {
			lines = append(lines, aliasLegend())
		}
		return strings.Join(lines, "\n")
	}
	bar := mutedStyle.Render("↑↓ select · ↵ open/run · esc back · ") +
		keycap("l") + keycap("u") + keycap("c") + keycap("i") + keycap("x") + keycap("n") + " " + keycap("delete") + mutedStyle.Render(" actions · ") +
		keycap("f5") + mutedStyle.Render(" refresh · ? help · ") + keycap("q") + mutedStyle.Render(" quit")
	if m.hasAlias() {
		bar += mutedStyle.Render(" · ") + accentStyle.Render("*") + mutedStyle.Render(" renamed")
	}
	return bar
}

// hasAlias reports whether any listed profile carries a custom label, so the
// "*" legend is only shown when it is actually relevant.
func (m Model) hasAlias() bool {
	for _, it := range m.list.Items() {
		if p, ok := it.(item); ok && p.label != "" && p.label != p.name {
			return true
		}
	}
	return false
}

// aliasLegend is the on-screen footnote explaining the "*" marker on renamed
// profiles: the display name is a custom label, not the profile's real slug.
func aliasLegend() string {
	return accentStyle.Render("*") + mutedStyle.Render(" renamed — display name differs from the profile slug")
}

// contextLine describes the current directory's relationship to profiles.
func contextLine(pwd string) string {
	if name, err := profile.Resolve("", pwd); err == nil {
		return fmt.Sprintf("This dir → %s", accentStyle.Render(name))
	}
	base := profile.SanitizeName(filepath.Base(pwd))
	conf := filepath.Join(config.ProfilesDir(), base+".conf")
	if _, err := os.Stat(conf); err == nil {
		return fmt.Sprintf("No .azprofile here. Link this dir to %s? (press u)", accentStyle.Render(base))
	}
	return fmt.Sprintf("No profile for this dir — create with: azrl login %s", accentStyle.Render(base))
}
