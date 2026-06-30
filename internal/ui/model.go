package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
)

type item struct{ name, tenant string }

func (i item) Title() string       { return i.name }
func (i item) Description() string { return i.tenant }
func (i item) FilterValue() string { return i.name }

// Model is the root TUI model.
type Model struct {
	list          list.Model
	spin          spinner.Model
	pwd           string
	width, height int
	status        string
	busy          bool
}

// NewModel builds the home model from the profiles on disk.
func NewModel() Model {
	pwd, _ := os.Getwd()
	var items []list.Item
	profs, _ := profile.List(config.ProfilesDir())
	for _, p := range profs {
		items = append(items, item{name: p.Name, tenant: p.Tenant})
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Profiles"
	l.SetShowHelp(false)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{list: l, spin: sp, pwd: pwd}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width-4, msg.Height-15)
	case opDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.status = failureStyle.Render("✗ " + msg.err.Error())
		} else {
			m.status = successStyle.Render("✓ " + msg.msg)
		}
		// refresh the list after a mutating action
		nm := NewModel()
		nm.width, nm.height, nm.status = m.width, m.height, m.status
		nm.list.SetSize(m.width-4, m.height-15)
		return nm, nil
	case spinner.TickMsg:
		var c tea.Cmd
		m.spin, c = m.spin.Update(msg)
		return m, c
	case tea.KeyMsg:
		if m.busy {
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			nm := NewModel()
			nm.width, nm.height = m.width, m.height
			nm.list.SetSize(m.width-4, m.height-15)
			return nm, nil
		case "u":
			base := profile.SanitizeName(filepathBase(m.pwd))
			m.busy = true
			m.status = ""
			return m, tea.Batch(m.spin.Tick, runUse(base))
		case "d":
			if it, ok := m.list.SelectedItem().(item); ok {
				m.busy = true
				m.status = ""
				return m, tea.Batch(m.spin.Tick, runDelete(it.name))
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m Model) View() string {
	ctx := panelStyle.Render(contextLine(m.pwd))
	statusLine := m.status
	if m.busy {
		statusLine = m.spin.View() + " working..."
	}
	help := mutedStyle.Render("u use · d delete · r refresh · q quit")
	return lipgloss.JoinVertical(lipgloss.Left, Banner(), "", ctx, "", m.list.View(), "", statusLine, help)
}

func filepathBase(p string) string { return filepath.Base(p) }

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
	return fmt.Sprintf("No profile for this dir — create with: azrl init %s", accentStyle.Render(base))
}
