package ui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// tab pairs a provider view with its tab-bar title. name is the provider's
// stable identifier for switchTabMsg matching ("" for the dashboard).
type tab struct {
	name  string
	title string
	model tea.Model
}

// tabsModel is the top-level tabbed container: it owns the alt-screen, draws a
// provider tab bar (Azure | GitHub | …), and delegates to the active tab. Tab
// switching is bound to '[' and ']' (both free in the inner views); every other
// key and all background messages flow to the tab(s).
type tabsModel struct {
	tabs     []tab
	active   int
	width    int
	height   int
	picker   *dirPicker
	options  *optionsPicker
	barFocus bool
	help     bool
}

// NewTabs builds the tabbed container on the dashboard (the default landing view).
func NewTabs() tabsModel {
	zone.NewGlobal() // idempotent — defensive so tests that skip runTabs still have a manager.
	return NewTabsOn(0)
}

// NewTabsOn builds the tabbed container preselected on tab index active. Tab 0 is
// the cross-provider dashboard; Azure leads the provider tabs (the flagship
// provider), followed by the rest in provider.All()'s order, each paired with its
// view by provider name (so the mapping is stable however providers sort). The
// dashboard keeps the full provider.All() list — its table sort is by last-used,
// independent of the tab strip order.
func NewTabsOn(active int) tabsModel {
	tabs := buildTabs("")
	if active < 0 || active >= len(tabs) {
		active = 0
	}
	return tabsModel{tabs: tabs, active: active}
}

// buildTabs assembles the dashboard plus a tab per enabled provider (the
// PROVIDERS key in azrl.conf; azure+github by default). extra names a
// provider to include regardless — e.g. the one a promoted binary opens on.
func buildTabs(extra string) []tab {
	enabled := map[string]bool{extra: true}
	for _, n := range config.EnabledProviders(config.ProfilesDir()) {
		enabled[n] = true
	}
	var provs []provider.Provider
	for _, p := range provider.All() {
		if enabled[p.Name()] {
			provs = append(provs, p)
		}
	}
	views := map[string]tea.Model{"azure": newAzureView(), "github": newGithubView(), "aws": newAwsView(), "gcp": newGcpView()}
	return append([]tab{{title: "Dashboard", model: newDashboard(provs)}}, providerTabs(preferredOrder(provs), views)...)
}

// preferredOrder arranges the tab strip as Azure, GitHub, AWS, Google Cloud,
// with any provider outside that list appended in registry order.
func preferredOrder(provs []provider.Provider) []provider.Provider {
	rank := map[string]int{"azure": 0, "github": 1, "aws": 2, "gcp": 3}
	out := make([]provider.Provider, 0, len(provs))
	var rest []provider.Provider
	for want := 0; want < len(rank); want++ {
		for _, p := range provs {
			if r, ok := rank[p.Name()]; ok && r == want {
				out = append(out, p)
			}
		}
	}
	for _, p := range provs {
		if _, ok := rank[p.Name()]; !ok {
			rest = append(rest, p)
		}
	}
	return append(out, rest...)
}

// providerTabs pairs each provider with its registered view by name. A provider
// with no view entry is skipped rather than paired with a nil tea.Model, which
// would nil-panic in Update/View — this guards future providers (GCP, …) added
// to provider.All() before their view lands here.
func providerTabs(provs []provider.Provider, views map[string]tea.Model) []tab {
	var tabs []tab
	for _, p := range provs {
		mdl, ok := views[p.Name()]
		if !ok {
			continue
		}
		tabs = append(tabs, tab{name: p.Name(), title: p.Title(), model: mdl})
	}
	return tabs
}

// NewTabsForProvider builds the tabbed container preselected on the tab whose
// provider Name() matches name (falling back to the dashboard).
func NewTabsForProvider(name string) tabsModel {
	m := tabsModel{tabs: buildTabs(name)}
	for i, t := range m.tabs {
		if t.name == name {
			m.active = i
			break
		}
	}
	return m
}

func (m tabsModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, t := range m.tabs {
		if c := t.model.Init(); c != nil {
			cmds = append(cmds, c)
		}
	}
	return tea.Batch(cmds...)
}

// inputCapturer marks a tab whose current state consumes raw keystrokes (e.g.
// a rename text input); while active the container forwards tab-switch keys
// instead of handling them.
type inputCapturer interface{ capturesInput() bool }

// activeCapturesInput reports whether the active tab is in a text-entry state.
func (m tabsModel) activeCapturesInput() bool {
	if c, ok := m.tabs[m.active].model.(inputCapturer); ok {
		return c.capturesInput()
	}
	return false
}

func (m tabsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Reserve the banner rows, the gap under it, and the tab-bar line so each
		// tab's own frame fits.
		return m.broadcast(tea.WindowSizeMsg{Width: msg.Width, Height: m.innerHeight()})
	case tea.KeyMsg:
		// The help overlay swallows its closing keypress.
		if m.help {
			m.help = false
			return m, nil
		}
		// While the tab bar holds focus, ←/→ walk the tabs and ↓/enter/esc
		// hand focus back to the active view.
		if m.barFocus && m.picker == nil {
			switch msg.String() {
			case "left", "shift+tab", "[":
				m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
				return m, nil
			case "right", "tab", "]":
				m.active = (m.active + 1) % len(m.tabs)
				return m, nil
			case "down", "enter", "esc":
				m.barFocus = false
				return m.broadcast(barFocusMsg{focused: false})
			case "d":
				pk := newDirPicker(m.width, m.innerHeight())
				m.picker = &pk
				return m, nil
			case "o":
				op := newOptionsPicker(m.width, m.innerHeight())
				m.options = &op
				return m, nil
			case "?":
				m.help = true
				return m, nil
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}
		// The options overlay owns every key while open.
		if m.options != nil {
			no, saved, closed := m.options.update(msg)
			m.options = &no
			if closed {
				m.options = nil
				if len(saved) > 0 {
					return m.applyProviderSelection(saved)
				}
			}
			return m, nil
		}
		// The change-directory overlay owns every key while open.
		if m.picker != nil {
			np, picked, closed := m.picker.update(msg)
			m.picker = &np
			if closed {
				m.picker = nil
				if picked != "" && os.Chdir(picked) == nil {
					return m.broadcast(cwdChangedMsg{dir: picked})
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "]", "tab":
			if !m.activeCapturesInput() {
				m.active = (m.active + 1) % len(m.tabs)
				return m, nil
			}
		case "[", "shift+tab":
			if !m.activeCapturesInput() {
				m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
				return m, nil
			}
		case "d":
			if !m.activeCapturesInput() {
				pk := newDirPicker(m.width, m.innerHeight())
				m.picker = &pk
				return m, nil
			}
		case "o":
			if !m.activeCapturesInput() {
				op := newOptionsPicker(m.width, m.innerHeight())
				m.options = &op
				return m, nil
			}
		case "?":
			if !m.activeCapturesInput() {
				m.help = true
				return m, nil
			}
		}
		nm, c := m.tabs[m.active].model.Update(msg)
		m.tabs[m.active].model = nm
		return m, c
	case focusTabsMsg:
		m.barFocus = true
		return m.broadcast(barFocusMsg{focused: true})
	case switchTabMsg:
		for i, t := range m.tabs {
			if t.name == msg.provider {
				m.active = i
				nm, c := m.tabs[i].model.Update(msg)
				m.tabs[i].model = nm
				return m, c
			}
		}
		return m, nil
	default:
		// Background messages (spinner ticks, identity, op-done) can belong to any
		// tab; forward to all so each tab's own async work resolves.
		return m.broadcast(msg)
	}
}

// leftRelease reports a completed left click (bubblezone's canonical event).
func leftRelease(msg tea.MouseMsg) bool {
	return msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft
}

// handleMouse routes mouse input: overlays swallow everything (later task makes
// them mouse-aware), tab cells switch tabs, everything else is the active tab's
// business.
func (m tabsModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.help || m.options != nil || m.picker != nil {
		return m, nil
	}
	if leftRelease(msg) {
		for i := range m.tabs {
			if z := zone.Get(fmt.Sprintf("tab:%d", i)); z != nil && z.InBounds(msg) {
				m.active = i
				m.barFocus = false
				return m, nil
			}
		}
	}
	// Forward to the active tab (wheel + row clicks handled there in later tasks).
	return m.forwardMouse(msg)
}

// forwardMouse hands the event to the active tab's model, mirroring how key
// messages already reach it.
func (m tabsModel) forwardMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	nm, c := m.tabs[m.active].model.Update(msg)
	m.tabs[m.active].model = nm
	return m, c
}

// Close tears down any tab model that owns OS resources by calling its Close if
// it implements one (e.g. the dashboard's fsnotify watcher). It is best-effort:
// per-tab errors are ignored. run.go calls it once after the program loop ends,
// so teardown happens on every quit path regardless of the active tab or the
// quit key — the alternative of closing in a tab's Update would leak whenever
// the container intercepts the quit key or another tab is active.
func (m tabsModel) Close() error {
	for _, t := range m.tabs {
		if c, ok := t.model.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}
	return nil
}

// broadcast forwards msg to every tab, collecting their commands.
func (m tabsModel) broadcast(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	for i := range m.tabs {
		nm, c := m.tabs[i].model.Update(msg)
		m.tabs[i].model = nm
		if c != nil {
			cmds = append(cmds, c)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m tabsModel) View() string {
	var cells []string
	for i, t := range m.tabs {
		label := " " + t.title + " "
		var styled string
		switch {
		case i == m.active && m.barFocus:
			// The bar holds focus: bright selection block.
			styled = selBlockActive.Render(label)
		case i == m.active:
			// Focus lives below: the tab retains its selection, dimmed.
			styled = selBlockParent.Render(label)
		default:
			styled = inactiveTabStyle.Render(label)
		}
		cells = append(cells, zone.Mark(fmt.Sprintf("tab:%d", i), styled))
	}
	bar := strings.Join(cells, tabSepStyle.Render("│"))
	// Center the banner block within the full terminal width (each line kept ≤
	// width; centering only pads with spaces).
	banner := bannerFor(m.width)
	if m.width > 0 {
		banner = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, banner)
	}
	body := m.tabs[m.active].model.View()
	if m.picker != nil {
		body = m.picker.view()
	}
	if m.options != nil {
		// Settings float as a centered popup over whatever is beneath.
		body = overlayCenter(body, m.options.view(), m.width)
	}
	if m.help {
		body = overlayCenter(body, helpOverlay(), m.width)
	}
	out := banner + "\n\n" + bar + "\n" + body
	// Backstop invariant: no line may exceed the terminal width, whatever a child
	// renders. Truncate every line (ANSI-aware) to guarantee it.
	if m.width > 0 {
		lines := strings.Split(out, "\n")
		for i, l := range lines {
			lines[i] = truncateLine(l, m.width)
		}
		out = strings.Join(lines, "\n")
	}
	return zone.Scan(out)
}

var (
	inactiveTabStyle = lipgloss.NewStyle().Foreground(gray)
	tabSepStyle      = lipgloss.NewStyle().Foreground(azureDeep)
)

// cwdChangedMsg is broadcast after the change-directory overlay applies
// os.Chdir, so every tab re-aggregates against the new working directory.
type cwdChangedMsg struct{ dir string }

// focusTabsMsg is emitted by a view when ↑ is pressed at the top of its list,
// handing keyboard focus to the tab bar.
type focusTabsMsg struct{}

// barFocusMsg tells the views whether the tab bar holds focus, so their own
// selections dim to the parent shade while it does.
type barFocusMsg struct{ focused bool }

// innerHeight is the space below the banner, its gap, and the tab bar.
func (m tabsModel) innerHeight() int {
	h := m.height - lipgloss.Height(bannerFor(m.width)) - 2
	if h < 0 {
		h = 0
	}
	return h
}

// applyProviderSelection persists the chosen provider set and rebuilds the
// tab strip around it: old tab resources (the dashboard's fsnotify watcher)
// are closed, fresh views are constructed, sized, and initialised, and the
// active tab clamps to the dashboard when its provider was disabled.
func (m tabsModel) applyProviderSelection(names []string) (tea.Model, tea.Cmd) {
	_ = config.SetEnabledProviders(config.ProfilesDir(), names)
	current := m.tabs[m.active].name
	_ = m.Close()
	m.tabs = buildTabs("")
	m.active = 0
	for i, t := range m.tabs {
		if t.name == current {
			m.active = i
			break
		}
	}
	var cmds []tea.Cmd
	for _, t := range m.tabs {
		if c := t.model.Init(); c != nil {
			cmds = append(cmds, c)
		}
	}
	nm, c := m.broadcast(tea.WindowSizeMsg{Width: m.width, Height: m.innerHeight()})
	if c != nil {
		cmds = append(cmds, c)
	}
	return nm, tea.Batch(cmds...)
}

// helpOverlay is the full keymap reference, floated over any tab by '?'.
func helpOverlay() string {
	lines := []string{
		paneTitleStyle.Render("KEYS"),
		"",
		keyHelp("↑↓", "select", "↵", "open/run", "esc", "back"),
		keyHelp("⇥ ]", "next tab", "⇧⇥ [", "prev tab"),
		keyHelp("s", "sign in", "u", "link here", "n", "new profile", "t", "shell as profile"),
		keyHelp("c", "open console", "a", "capture (empty state) · adopt (dashboard)", "b", "browser profile", "delete", "remove"),
		keyHelp("e", "write .envrc (azure)", "d", "change dir", "o", "options"),
		keyHelp("r f5", "refresh", "w", "recheck drift (dashboard)", "?", "close help"),
		keyHelp("q", "quit"),
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(azureBlue).
		Padding(1, 3).
		Render(strings.Join(lines, "\n"))
}
