package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// dashboardRow is one profile line: its owning provider plus the disk-only Status.
type dashboardRow struct {
	providerName  string
	providerTitle string
	status        provider.Status
}

// dashboardTickMsg drives the periodic disk re-read.
type dashboardTickMsg struct{}

// switchTabMsg asks the tab container to jump to a provider's tab with a profile
// pre-selected; emitted when the user presses Enter on a dashboard row.
type switchTabMsg struct {
	provider string
	profile  string
}

// dashboardModel is the top-level "who am I, everywhere" view: it aggregates
// every provider's profiles into one table, sorted by last-used, and refreshes
// from disk on a timer. It never makes a network call.
type dashboardModel struct {
	providers []provider.Provider
	rows      []dashboardRow
	cursor    int
	width     int
	height    int
	interval  time.Duration
}

// newDashboard builds the dashboard over provs, reading the poll interval from
// azrl.conf and aggregating the initial rows from disk.
func newDashboard(provs []provider.Provider) dashboardModel {
	m := dashboardModel{
		providers: provs,
		interval:  time.Duration(config.DashboardPollSecs(config.ProfilesDir())) * time.Second,
	}
	m.rows = aggregate(provs)
	return m
}

// aggregate flattens every provider's profiles into rows sorted by LastUsed
// descending (zero time last). A per-profile Status error is fault-isolated to
// its own row.
func aggregate(provs []provider.Provider) []dashboardRow {
	var rows []dashboardRow
	for _, ps := range provider.Collect(provs) {
		for _, st := range ps.Statuses {
			rows = append(rows, dashboardRow{providerName: ps.Name, providerTitle: ps.Title, status: st})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].status.LastUsed.After(rows[j].status.LastUsed)
	})
	return rows
}

func (m dashboardModel) Init() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg { return dashboardTickMsg{} })
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case dashboardTickMsg:
		m.rows = aggregate(m.providers)
		return m, tea.Tick(m.interval, func(time.Time) tea.Msg { return dashboardTickMsg{} })
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r", "w":
			m.rows = aggregate(m.providers)
			return m, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
			return m, nil
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.rows) {
				row := m.rows[m.cursor]
				return m, func() tea.Msg {
					return switchTabMsg{provider: row.providerName, profile: row.status.ProfileName}
				}
			}
		}
	}
	return m, nil
}

func (m dashboardModel) View() string {
	header := paneTitleStyle.Render("Dashboard") + mutedStyle.Render(" — who am I, everywhere")
	help := mutedStyle.Render("↑↓ select · ↵ open tab · r refresh · w recheck drift · [ ] tab · q quit")
	if len(m.rows) == 0 {
		empty := mutedStyle.Render("No profiles yet. Create one with `azrl init <name>` or `ghrl login <name>`.")
		return frameStyle.Render(strings.Join([]string{header, "", empty, "", help}, "\n"))
	}

	cols := []string{"Provider", "Profile", "Identity", "Dir", "Expiry", "Drift", "Last used"}
	matrix := [][]string{cols}
	for _, r := range m.rows {
		matrix = append(matrix, []string{
			r.providerTitle,
			r.status.ProfileName,
			orDash(r.status.Identity),
			orDash(shortDir(r.status.Directory)),
			expiryText(r.status.Expiry),
			driftText(r.status.Drifted),
			lastUsedText(r.status.LastUsed),
		})
	}
	widths := make([]int, len(cols))
	for _, row := range matrix {
		for i, c := range row {
			if w := lipgloss.Width(c); w > widths[i] {
				widths[i] = w
			}
		}
	}

	var b strings.Builder
	for ri, row := range matrix {
		var cells []string
		for i, c := range row {
			cells = append(cells, padTo(c, widths[i]))
		}
		line := strings.Join(cells, "  ")
		if ri == 0 {
			b.WriteString("  " + paneTitleStyle.Render(line) + "\n")
			continue
		}
		marker := "  "
		if ri-1 == m.cursor {
			marker = accentStyle.Render("› ")
		}
		b.WriteString(marker + line + "\n")
	}

	return frameStyle.Render(strings.Join([]string{header, "", b.String(), help}, "\n"))
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

// driftText renders a loud marker when a row's ambient session has drifted.
func driftText(drifted bool) string {
	if drifted {
		return failureStyle.Render("⚠ drift")
	}
	return mutedStyle.Render("ok")
}

// expiryText renders a relative expiry ("in 42m" / "expired") with no network.
func expiryText(exp *time.Time) string {
	if exp == nil {
		return "—"
	}
	d := time.Until(*exp)
	if d <= 0 {
		return failureStyle.Render("expired")
	}
	return "in " + shortDur(d)
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

// lastUsedText renders a relative last-used time, or an em dash for the zero time.
func lastUsedText(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
