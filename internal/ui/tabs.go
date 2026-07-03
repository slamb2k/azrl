package ui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	barFocus bool
}

// NewTabs builds the tabbed container on the dashboard (the default landing view).
func NewTabs() tabsModel { return NewTabsOn(0) }

// NewTabsOn builds the tabbed container preselected on tab index active. Tab 0 is
// the cross-provider dashboard; Azure leads the provider tabs (the flagship
// provider), followed by the rest in provider.All()'s order, each paired with its
// view by provider name (so the mapping is stable however providers sort). The
// dashboard keeps the full provider.All() list — its table sort is by last-used,
// independent of the tab strip order.
func NewTabsOn(active int) tabsModel {
	provs := provider.All()
	views := map[string]tea.Model{"azure": NewModel(), "github": newGithubView(), "aws": newAwsView(), "gcp": newGcpView()}
	tabs := append([]tab{{title: "Dashboard", model: newDashboard(provs)}}, providerTabs(preferredOrder(provs), views)...)
	if active < 0 || active >= len(tabs) {
		active = 0
	}
	return tabsModel{tabs: tabs, active: active}
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
	m := NewTabsOn(0)
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
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Reserve the banner rows, the gap under it, and the tab-bar line so each
		// tab's own frame fits.
		innerH := msg.Height - lipgloss.Height(bannerFor(msg.Width)) - 2
		if innerH < 0 {
			innerH = 0
		}
		inner := tea.WindowSizeMsg{Width: msg.Width, Height: innerH}
		return m.broadcast(inner)
	case tea.KeyMsg:
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
			case "q", "ctrl+c":
				return m, tea.Quit
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
				innerH := m.height - lipgloss.Height(bannerFor(m.width)) - 2
				pk := newDirPicker(m.width, innerH)
				m.picker = &pk
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
		switch {
		case i == m.active && m.barFocus:
			// The bar holds focus: bright selection block.
			cells = append(cells, selBlockActive.Render(label))
		case i == m.active:
			// Focus lives below: the tab retains its selection, dimmed.
			cells = append(cells, selBlockParent.Render(label))
		default:
			cells = append(cells, inactiveTabStyle.Render(label))
		}
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
	return out
}

var (
	activeTabStyle   = lipgloss.NewStyle().Foreground(gold).Bold(true).Underline(true)
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
