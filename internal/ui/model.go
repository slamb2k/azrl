package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/list"
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
	pwd           string
	width, height int
	status        string
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
	return Model{list: l, pwd: pwd}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width-4, msg.Height-14)
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m Model) View() string {
	ctx := panelStyle.Render(contextLine(m.pwd))
	help := mutedStyle.Render("enter use · l login · i init · c capture · u use · d delete · r refresh · q quit")
	return lipgloss.JoinVertical(lipgloss.Left, Banner(), "", ctx, "", m.list.View(), "", help)
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
	return fmt.Sprintf("No profile for this dir. Create one named %s? (press i)", accentStyle.Render(base))
}
