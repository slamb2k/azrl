package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/fsnotify/fsnotify"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// dashItem is one selectable row of the landing view, in render order across
// the three sections. adoptDir is set on unmanaged mapping rows so [a] can
// launch the provider's capture flow for that directory.
type dashItem struct {
	provider string
	profile  string
	adoptDir string
}

// dashboardTickMsg drives the periodic disk re-read (the fallback poll).
type dashboardTickMsg struct{}

// fsEventMsg fires when a watched provider dir changes on disk (an external
// az/gh/aws/gcloud token cache write), driving an immediate re-aggregate.
type fsEventMsg struct{}

// switchTabMsg asks the tab container to jump to a provider's tab with a profile
// pre-selected; emitted when the user presses Enter on a dashboard row.
type switchTabMsg struct {
	provider string
	profile  string
}

// dashboardModel is the landing view: MAPPINGS (directory→profile associations
// with scope and drift markers), AMBIENT (each provider's native default), and
// UNMAPPED PROFILES (saved profiles no mapping names). It refreshes from disk
// on a timer and on fsnotify events, and never makes a network call.
type dashboardModel struct {
	providers []provider.Provider
	ov        Overview
	items     []dashItem
	cursor    int
	width     int
	height    int
	interval  time.Duration
	watcher   *fsnotify.Watcher
}

// newDashboard builds the dashboard over provs, reading the poll interval from
// azrl.conf and aggregating the initial sections from disk. It also creates a
// best-effort filesystem watcher over each provider's WatchDirs so external
// token changes refresh the view immediately; on watcher-create failure it
// falls back to timer-only polling (watcher stays nil).
func newDashboard(provs []provider.Provider) dashboardModel {
	m := dashboardModel{
		providers: provs,
		interval:  time.Duration(config.DashboardPollSecs(config.ProfilesDir())) * time.Second,
	}
	m.reload()
	m.watcher = newDashboardWatcher(provs)
	return m
}

// reload re-aggregates the three sections from disk and rebuilds the flat
// selectable-item list, clamping the cursor to the new bounds.
func (m *dashboardModel) reload() {
	cwd, _ := os.Getwd()
	m.ov = BuildOverview(m.providers, cwd)
	m.items = overviewItems(m.ov)
	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// overviewItems flattens the overview into selectable items in the exact order
// View renders them: mappings, then ambient, then unmapped.
func overviewItems(ov Overview) []dashItem {
	var items []dashItem
	for _, r := range ov.Mappings {
		it := dashItem{provider: r.Provider, profile: r.Profile}
		if r.Unmanaged != "" {
			it.adoptDir = r.Dir
		}
		items = append(items, it)
	}
	for _, r := range ov.Ambient {
		items = append(items, dashItem{provider: r.Provider, profile: r.Profile})
	}
	for _, r := range ov.Unmapped {
		items = append(items, dashItem{provider: r.Provider, profile: r.Status.ProfileName})
	}
	return items
}

// newDashboardWatcher creates an fsnotify watcher and adds every provider
// WatchDir (fsnotify is not recursive, so WatchDirs already enumerates the
// per-profile subdirs). It returns nil on create failure so the dashboard falls
// back to timer-only polling. Individual Add failures are ignored (best-effort).
func newDashboardWatcher(provs []provider.Provider) *fsnotify.Watcher {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil
	}
	for _, p := range provs {
		for _, d := range p.WatchDirs() {
			_ = w.Add(d)
		}
	}
	return w
}

// watchCmd blocks on the watcher's Events/Errors channels and returns an
// fsEventMsg on any activity, so Update can re-aggregate and re-arm. It returns
// nil when the watcher is absent or its channels have closed (stopping the loop).
func watchCmd(w *fsnotify.Watcher) tea.Cmd {
	if w == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case _, ok := <-w.Events:
			if !ok {
				return nil
			}
			return fsEventMsg{}
		case _, ok := <-w.Errors:
			if !ok {
				return nil
			}
			return fsEventMsg{}
		}
	}
}

func (m dashboardModel) Init() tea.Cmd {
	tick := tea.Tick(m.interval, func(time.Time) tea.Msg { return dashboardTickMsg{} })
	if wc := watchCmd(m.watcher); wc != nil {
		return tea.Batch(tick, wc)
	}
	return tick
}

// Close releases the dashboard's OS resources — currently the fsnotify watcher.
// It is best-effort and safe to call when the watcher is nil or already closed
// (fsnotify's "already closed" error is ignored), so centralized teardown in
// run.go can call it on every quit path without guarding. A pointer field, so
// the final model returned by tea.Program.Run holds the live watcher.
func (m dashboardModel) Close() error {
	if m.watcher != nil {
		_ = m.watcher.Close()
	}
	return nil
}

// adoptArgs maps an unmanaged mapping row to the azrl subcommand that captures
// the current session into a new profile named after the directory (Azure's
// capture is top-level; the other providers sit under their command group).
func adoptArgs(providerName, dir string) []string {
	name := profile.DefaultName("", dir)
	switch providerName {
	case "azure":
		return []string{"capture", name}
	case "github":
		return groupArgs("gh", "capture", name)
	default:
		return []string{providerName, "capture", name}
	}
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case dashboardTickMsg:
		m.reload()
		return m, tea.Tick(m.interval, func(time.Time) tea.Msg { return dashboardTickMsg{} })
	case fsEventMsg:
		// An external token/config change on a watched dir: re-aggregate now (disk
		// only, cheap) and re-arm the watch. The timer keeps running as a fallback.
		m.reload()
		return m, watchCmd(m.watcher)
	case cwdChangedMsg:
		m.reload()
		return m, nil
	case opDoneMsg:
		// A handed-off flow (e.g. adopt → capture) finished: pick up its writes.
		m.reload()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "f5", "w":
			m.reload()
			return m, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				return m, nil
			}
			// Already at the top: hand focus to the tab bar.
			return m, func() tea.Msg { return focusTabsMsg{} }
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
			return m, nil
		case "a":
			if m.cursor >= 0 && m.cursor < len(m.items) {
				if it := m.items[m.cursor]; it.adoptDir != "" {
					return m, runHandoff(adoptArgs(it.provider, it.adoptDir))
				}
			}
			return m, nil
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.items) {
				it := m.items[m.cursor]
				return m, func() tea.Msg {
					return switchTabMsg{provider: it.provider, profile: it.profile}
				}
			}
		}
	}
	return m, nil
}

func (m dashboardModel) View() string {
	cwd, _ := os.Getwd()
	// The same top-line anatomy as the provider tabs: title left, dir centered,
	// the next-action hint right — justified, not dot-separated.
	contentW := m.width - 4
	if contentW < 1 {
		contentW = 1
	}
	short, notice := dashboardHints(m.ov)
	header := justify(contentW, "🧭 "+paneTitleStyle.Render("Dashboard"),
		"📁 "+displayDir(cwd), short)
	help := keyHelpFit(m.width-4,
		[]string{"↑↓", "select", "↵", "open tab", "a", "adopt"},
		[]string{"q", "quit", "f5", "refresh", "w", "recheck drift", "⇥", "tab", "d", "dir", "o", "options"})

	var body []string
	if notice != "" {
		// The full explanation wraps beneath the header — the right zone only
		// carries the compact chip.
		body = append(body, strings.Split(ansi.Wordwrap(notice, contentW, ""), "\n")...)
		body = append(body, "")
	}
	idx := 0
	marker := func() string {
		s := "  "
		if idx == m.cursor {
			s = accentStyle.Render("› ")
		}
		idx++
		return s
	}

	body = append(body, paneTitleStyle.Render("MAPPINGS"))
	if len(m.ov.Mappings) == 0 {
		body = append(body, "  "+mutedStyle.Render("No mappings yet — link a directory with `azrl use <name>`."))
	} else {
		dirW, tgtW := mappingWidths(m.ov.Mappings)
		for _, r := range m.ov.Mappings {
			body = append(body, marker()+mappingLine(r, dirW, tgtW))
		}
	}

	body = append(body, "", paneTitleStyle.Render("AMBIENT")+mutedStyle.Render(" — defaults in effect"))
	if len(m.ov.Ambient) == 0 {
		body = append(body, "  "+mutedStyle.Render("No native defaults detected."))
	} else {
		titleW, idW, srcW := ambientWidths(m.ov.Ambient)
		for _, r := range m.ov.Ambient {
			body = append(body, marker()+ambientLine(r, titleW, idW, srcW))
		}
	}

	body = append(body, "", paneTitleStyle.Render("UNMAPPED PROFILES"))
	switch {
	case len(m.ov.Unmapped) > 0:
		for _, r := range m.ov.Unmapped {
			body = append(body, marker()+unmappedLine(r))
		}
	case m.ov.hasProfiles():
		body = append(body, "  "+mutedStyle.Render("All profiles are mapped."))
	default:
		body = append(body, "  "+mutedStyle.Render("No profiles yet. Create one with `azrl login <name>` or `ghrl login <name>`."))
	}

	return m.frame(header, body, help)
}

// scopeMarker renders the mapping's cwd relationship with the same colour
// ramp as the profile tabs: ● green set here, ● orange inherited from the
// nearest governing ancestor, ● dark-white mapped elsewhere (not governing
// the cwd).
func scopeMarker(scope string) string {
	switch scope {
	case ScopeCwd:
		return successStyle.Render("●")
	case ScopeAncestor:
		return lipgloss.NewStyle().Foreground(goldDeep).Render("●")
	}
	return lipgloss.NewStyle().Foreground(whiteDim).Render("●")
}

// sourceIcon renders where the mapping comes from: the provider's pointer
// filename, or "(git)" for a repo-local git-config association.
func sourceIcon(r MappingRow) string {
	if r.Source == "gitconfig" {
		return "(git)"
	}
	return r.Pointer
}

// mappingTarget renders the mapping's right-hand side: provider:profile for a
// managed row, or the accented unmanaged identity.
func mappingTarget(r MappingRow) string {
	if r.Unmanaged != "" {
		return r.Provider + ": " + accentStyle.Render(r.Unmanaged)
	}
	return r.Provider + ":" + r.Profile
}

// mappingWidths measures the dir and target columns so mapping rows align.
func mappingWidths(rows []MappingRow) (dirW, tgtW int) {
	for _, r := range rows {
		if w := lipgloss.Width(shortDir(r.Dir)); w > dirW {
			dirW = w
		}
		if w := lipgloss.Width(mappingTarget(r)); w > tgtW {
			tgtW = w
		}
	}
	return dirW, tgtW
}

// mappingLine renders one MAPPINGS row: scope marker, dir → target, source
// icon, then the drift/conflict/adopt annotations.
func mappingLine(r MappingRow, dirW, tgtW int) string {
	line := scopeMarker(r.Scope) + " " + padTo(shortDir(r.Dir), dirW) + " → " +
		padTo(mappingTarget(r), tgtW) + "  " + mutedStyle.Render(sourceIcon(r))
	if r.Conflict != nil {
		line += "  " + failureStyle.Render("⚠ conflict") +
			mutedStyle.Render(" "+r.Pointer+" → "+r.Conflict.PointerProfile+" (git config wins)")
	}
	if r.Drifted {
		line += "  " + failureStyle.Render("⚠ drift")
	}
	if expired(r.Expiry) {
		line += "  " + failureStyle.Render("⚠ expired")
	}
	if r.Unmanaged != "" {
		line += "  " + accentStyle.Render("unmanaged") + mutedStyle.Render(" · [a]dopt")
	}
	return line
}

// ambientWidths measures the ambient columns so the 🌐 rows align.
func ambientWidths(rows []AmbientRow) (titleW, idW, srcW int) {
	for _, r := range rows {
		if w := lipgloss.Width(r.Title); w > titleW {
			titleW = w
		}
		if w := lipgloss.Width(r.Identity); w > idW {
			idW = w
		}
		if w := lipgloss.Width(r.Source); w > srcW {
			srcW = w
		}
	}
	return titleW, idW, srcW
}

// ambientLine renders one AMBIENT row: 🌐 provider, identity, winning source,
// and the matching profile (or an explicit unmanaged label). Never a drift
// marker — ambient is the global fallback, not a mapping.
func ambientLine(r AmbientRow, titleW, idW, srcW int) string {
	line := "🌐 " + padTo(r.Title, titleW) + "  " + padTo(r.Identity, idW) + "  " +
		padTo(mutedStyle.Render(r.Source), srcW) + "  "
	if r.Profile != "" {
		// The default isn't associated with any folder, so no profile/dir
		// target — just whether azrl manages this identity.
		return line + successStyle.Render("managed")
	}
	return line + accentStyle.Render("unmanaged")
}

// unmappedLine renders one muted UNMAPPED PROFILES row: provider:name ·
// identity · expiry (the expiry keeps its warning styling).
func unmappedLine(r UnmappedRow) string {
	st := r.Status
	// The deep-grey ● matches the profile tabs' mapped-nowhere tier.
	return lipgloss.NewStyle().Foreground(grayDeep).Render("●") + " " +
		mutedStyle.Render(r.Provider+":"+st.ProfileName+" · "+orDash(st.Identity)+" · ") + expiryText(st.Expiry)
}

// frame assembles the dashboard content and fills it to the full terminal width
// and height: header, a blank row, the body, blank filler rows so the footer
// sits near the terminal bottom, then the footer — every line padded to the
// content width (so the frame spans edge-to-edge) and truncated so none overflow.
func (m dashboardModel) frame(header string, body []string, footer string) string {
	contentW := m.width - 4
	if m.width <= 0 || contentW < 1 {
		contentW = 1
	}
	lines := append([]string{header, rule(contentW), ""}, body...)
	// Reserve the frame border (2 rows) and the footer row, then pad the middle so
	// the footer lands at the bottom instead of a short box with dead space below.
	for len(lines) < m.height-2-1 {
		lines = append(lines, "")
	}
	// The help bar centers as a group, matching the provider tabs' frame.
	lines = append(lines, lipgloss.PlaceHorizontal(contentW, lipgloss.Center, footer))
	for i, l := range lines {
		lines[i] = padTo(truncateLine(l, contentW), contentW)
	}
	return frameStyle.Render(strings.Join(lines, "\n"))
}

// orDash renders a blank cell as an em dash.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// shortDir abbreviates the home prefix to "~" for compactness.
func shortDir(dir string) string {
	if dir == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(dir, home) {
		return "~" + strings.TrimPrefix(dir, home)
	}
	return dir
}

// expiryText renders a relative expiry ("in 42m" / "expired") with no network.
func expiryText(exp *time.Time) string {
	if exp == nil {
		return "—"
	}
	if expired(exp) {
		return failureStyle.Render("expired")
	}
	return "in " + shortDur(time.Until(*exp))
}

// expired reports whether a cached expiry timestamp is in the past; a nil
// (none/unknown) expiry is never expired.
func expired(exp *time.Time) bool {
	return exp != nil && time.Until(*exp) <= 0
}

func shortDur(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// dashboardHints picks the next most useful action by priority (conflict >
// drift > expired governing link > unmanaged > expired unmapped > first-link
// nudge > all good) and returns two
// renderings: a compact chip that fits the header's right zone, and a full
// explanation for the notice line beneath ("" when nothing needs attention).
func dashboardHints(ov Overview) (short, notice string) {
	for _, r := range ov.Mappings {
		if r.Conflict != nil {
			return failureStyle.Render("⚠ conflict in " + shortDir(r.Dir)),
				failureStyle.Render("⚠ conflict in "+shortDir(r.Dir)) +
					mutedStyle.Render(" — git config says ") + accentStyle.Render(r.Conflict.GitConfigUser) +
					mutedStyle.Render(" ("+r.Conflict.GitConfigProfile+") but "+r.Pointer+" says ") +
					accentStyle.Render(r.Conflict.PointerProfile) + mutedStyle.Render(" — git config wins; fix the pointer")
		}
	}
	for _, r := range ov.Mappings {
		if r.Drifted {
			shell := ""
			for _, a := range ov.Ambient {
				if a.Provider == r.Provider {
					shell = a.Identity
				}
			}
			side := mutedStyle.Render(" — the shell has no session; the link expects ") + accentStyle.Render(r.Profile)
			if shell != "" {
				side = mutedStyle.Render(" — the shell would act as ") + accentStyle.Render(shell) +
					mutedStyle.Render(" but this directory is linked to ") + accentStyle.Render(r.Profile)
			}
			return failureStyle.Render("⚠ drift in " + shortDir(r.Dir)),
				failureStyle.Render("⚠ drift in "+shortDir(r.Dir)) + side +
					mutedStyle.Render(" · ") + keycap("↵") + mutedStyle.Render(" opens its tab to fix")
		}
	}
	// An expired link that governs the cwd means the next CLI command here will
	// hit a wall — more urgent than adoptable identities, less than conflict/drift.
	for _, r := range ov.Mappings {
		if r.Scope != ScopeNone && expired(r.Expiry) {
			return failureStyle.Render("⚠ " + r.Provider + ":" + r.Profile + " expired"),
				accentStyle.Render(r.Provider+":"+r.Profile) + mutedStyle.Render(" is linked here but its session has expired — ") +
					keycap("↵") + mutedStyle.Render(" opens its tab to sign in")
		}
	}
	for _, r := range ov.Mappings {
		if r.Unmanaged != "" {
			return accentStyle.Render("unmanaged identity"),
				accentStyle.Render(r.Unmanaged) + mutedStyle.Render(" in "+shortDir(r.Dir)+" is unmanaged — ") +
					keycap("a") + mutedStyle.Render(" adopts it into a profile")
		}
	}
	for _, u := range ov.Unmapped {
		if expired(u.Status.Expiry) {
			return failureStyle.Render("⚠ " + u.Provider + ":" + u.Status.ProfileName + " expired"),
				accentStyle.Render(u.Provider+":"+u.Status.ProfileName) + mutedStyle.Render(" has expired — ") +
					keycap("↵") + mutedStyle.Render(" opens its tab to sign in")
		}
	}
	if len(ov.Mappings) == 0 {
		return mutedStyle.Render("no directories linked yet"), ""
	}
	return mutedStyle.Render("all good · ") + keycap("↵") + mutedStyle.Render(" drills in · ") + keycap("d") + mutedStyle.Render(" changes dir"), ""
}
