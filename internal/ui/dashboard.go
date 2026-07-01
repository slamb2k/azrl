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

	// Fit the table to the terminal: drop lower-priority columns as width shrinks,
	// then truncate the Identity cell if the surviving columns still overflow.
	active, widths := m.fitColumns(matrix)

	var b strings.Builder
	for ri, row := range matrix {
		var cells []string
		for _, ci := range active {
			cells = append(cells, padTo(row[ci], widths[ci]))
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

// Dashboard column indices into the full matrix row.
const (
	colProvider = 0
	colProfile  = 1
	colIdentity = 2
	colDir      = 3
	colExpiry   = 4
	colDrift    = 5
	colLastUsed = 6
)

// dashDropOrder lists the droppable columns in the order they are shed as the
// terminal narrows; Provider, Profile, Identity and Drift are never dropped.
var dashDropOrder = []int{colLastUsed, colExpiry, colDir}

// fitColumns picks which columns survive at the current width and their pixel
// widths, dropping low-priority columns then truncating the Identity cells (in
// place) when the survivors still overflow. It returns the active column indices
// in display order and a per-column width map.
func (m dashboardModel) fitColumns(matrix [][]string) ([]int, map[int]int) {
	all := []int{colProvider, colProfile, colIdentity, colDir, colExpiry, colDrift, colLastUsed}

	// innerW is the room inside the frame border/padding for the marker + cells.
	innerW := m.width - 4
	if m.width <= 0 {
		innerW = 1 << 30
	}

	active := append([]int(nil), all...)
	widths := colWidths(matrix, active)
	for lineWidth(active, widths) > innerW && len(dashDropOrder) > 0 {
		next := -1
		for _, d := range dashDropOrder {
			if contains(active, d) {
				next = d
				break
			}
		}
		if next < 0 {
			break
		}
		active = remove(active, next)
		widths = colWidths(matrix, active)
	}

	// Still too wide: squeeze the Identity column with an ellipsis.
	if over := lineWidth(active, widths) - innerW; over > 0 && contains(active, colIdentity) {
		target := widths[colIdentity] - over
		if target < 3 {
			target = 3
		}
		for ri := range matrix {
			matrix[ri][colIdentity] = truncCell(matrix[ri][colIdentity], target)
		}
		widths = colWidths(matrix, active)
	}
	return active, widths
}

// colWidths measures the max visible width of each active column across matrix.
func colWidths(matrix [][]string, active []int) map[int]int {
	w := make(map[int]int, len(active))
	for _, row := range matrix {
		for _, ci := range active {
			if lw := lipgloss.Width(row[ci]); lw > w[ci] {
				w[ci] = lw
			}
		}
	}
	return w
}

// lineWidth is the rendered width of a data row: a 2-col marker, the cells, and
// a 2-col gap between them.
func lineWidth(active []int, widths map[int]int) int {
	total := 2
	for i, ci := range active {
		total += widths[ci]
		if i > 0 {
			total += 2
		}
	}
	return total
}

// truncCell trims s to width w, appending an ellipsis when it had to cut.
func truncCell(s string, w int) string {
	if w < 1 {
		w = 1
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 1 {
		return truncateLine(s, w)
	}
	return truncateLine(s, w-1) + "…"
}

func contains(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func remove(xs []int, x int) []int {
	out := xs[:0:0]
	for _, v := range xs {
		if v != x {
			out = append(out, v)
		}
	}
	return out
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
