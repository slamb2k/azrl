package ui

import (
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
	tabs   []tab
	active int
	width  int
	height int
}

// NewTabs builds the tabbed container on the dashboard (the default landing view).
func NewTabs() tabsModel { return NewTabsOn(0) }

// NewTabsOn builds the tabbed container preselected on tab index active. Tab 0 is
// the cross-provider dashboard; the provider tabs follow provider.All()'s order,
// each paired with its hand-zipped view.
func NewTabsOn(active int) tabsModel {
	provs := provider.All()
	views := []tea.Model{NewModel(), newGithubView()}
	tabs := []tab{{title: "Dashboard", model: newDashboard(provs)}}
	for i, p := range provs {
		var mdl tea.Model
		if i < len(views) {
			mdl = views[i]
		}
		tabs = append(tabs, tab{name: p.Name(), title: p.Title(), model: mdl})
	}
	if active < 0 || active >= len(tabs) {
		active = 0
	}
	return tabsModel{tabs: tabs, active: active}
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

func (m tabsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Reserve the banner rows and the tab-bar line so each tab's own frame fits.
		innerH := msg.Height - lipgloss.Height(bannerFor(msg.Width)) - 1
		if innerH < 0 {
			innerH = 0
		}
		inner := tea.WindowSizeMsg{Width: msg.Width, Height: innerH}
		return m.broadcast(inner)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "]":
			m.active = (m.active + 1) % len(m.tabs)
			return m, nil
		case "[":
			m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
			return m, nil
		}
		nm, c := m.tabs[m.active].model.Update(msg)
		m.tabs[m.active].model = nm
		return m, c
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
		if i == m.active {
			cells = append(cells, activeTabStyle.Render(label))
		} else {
			cells = append(cells, inactiveTabStyle.Render(label))
		}
	}
	bar := strings.Join(cells, tabSepStyle.Render("│"))
	out := bannerFor(m.width) + "\n" + bar + "\n" + m.tabs[m.active].model.View()
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
