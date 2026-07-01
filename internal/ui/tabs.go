package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// tab pairs a provider view with its tab-bar title.
type tab struct {
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

// NewTabs builds the tabbed container with one tab per provider, on the Azure tab.
func NewTabs() tabsModel { return NewTabsOn(0) }

// NewTabsOn builds the tabbed container preselected on tab index active.
func NewTabsOn(active int) tabsModel {
	tabs := []tab{
		{title: "Azure", model: NewModel()},
		{title: "GitHub", model: newGithubView()},
	}
	if active < 0 || active >= len(tabs) {
		active = 0
	}
	return tabsModel{tabs: tabs, active: active}
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
		// Reserve one line for the tab bar so each tab's own frame fits.
		inner := tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height - 1}
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
	return bar + "\n" + m.tabs[m.active].model.View()
}

var (
	activeTabStyle   = lipgloss.NewStyle().Foreground(gold).Bold(true).Underline(true)
	inactiveTabStyle = lipgloss.NewStyle().Foreground(gray)
	tabSepStyle      = lipgloss.NewStyle().Foreground(azureDeep)
)
