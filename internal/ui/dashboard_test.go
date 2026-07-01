package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/provider"
)

// seedDashHome points HOME at a temp dir with one Azure and one GitHub profile,
// each carrying a LAST_USED so the sort order is deterministic.
func seedDashHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(filepath.Join(az, "acme"), 0o755)
	os.WriteFile(filepath.Join(az, "acme.conf"),
		[]byte("AZ_TENANT=acme.com\nLAST_USED=2026-06-30T10:00:00Z\nLAST_DIR=/work/acme\n"), 0o644)
	os.WriteFile(filepath.Join(az, "acme", "azureProfile.json"),
		[]byte(`{"subscriptions":[{"user":{"name":"u@acme.com"},"isDefault":true,"tenantId":"g1"}]}`), 0o644)
	gh := filepath.Join(home, ".github-profiles")
	os.MkdirAll(filepath.Join(gh, "work"), 0o755)
	os.WriteFile(filepath.Join(gh, "work.conf"),
		[]byte("GH_HOST=github.com\nLAST_USED=2026-05-01T10:00:00Z\nLAST_DIR=/work/repo\n"), 0o644)
	os.WriteFile(filepath.Join(gh, "work", "hosts.yml"),
		[]byte("github.com:\n    user: octocat\n"), 0o644)
}

func sizedDashboard(t *testing.T) dashboardModel {
	t.Helper()
	m := newDashboard(provider.All())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	return nm.(dashboardModel)
}

func TestDashboardRendersRowsSortedByLastUsed(t *testing.T) {
	seedDashHome(t)
	m := sizedDashboard(t)
	v := m.View()
	for _, want := range []string{"Dashboard", "Azure", "GitHub", "acme", "work", "u@acme.com", "octocat@github.com"} {
		if !strings.Contains(v, want) {
			t.Fatalf("dashboard view missing %q:\n%s", want, v)
		}
	}
	// acme (2026-06-30) is more recent than work (2026-05-01) → sorts first.
	if strings.Index(v, "acme") > strings.Index(v, "octocat@github.com") {
		t.Fatalf("rows not sorted by last-used desc:\n%s", v)
	}
}

func TestDashboardEmptyState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.MkdirAll(filepath.Join(home, ".github-profiles"), 0o755)
	m := sizedDashboard(t)
	if !strings.Contains(m.View(), "No profiles yet") {
		t.Fatalf("expected empty state:\n%s", m.View())
	}
}

func TestDashboardEnterEmitsSwitchTab(t *testing.T) {
	seedDashHome(t)
	m := sizedDashboard(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter produced no command")
	}
	msg := cmd()
	sw, ok := msg.(switchTabMsg)
	if !ok {
		t.Fatalf("Enter did not emit switchTabMsg, got %T", msg)
	}
	// Cursor starts on the first (most-recent) row: the Azure acme profile.
	if sw.provider != "azure" || sw.profile != "acme" {
		t.Fatalf("switchTabMsg = %+v", sw)
	}
}

func TestDashboardTickReaggregates(t *testing.T) {
	seedDashHome(t)
	m := sizedDashboard(t)
	nm, cmd := m.Update(dashboardTickMsg{})
	if cmd == nil {
		t.Fatal("tick did not reschedule")
	}
	if len(nm.(dashboardModel).rows) != 2 {
		t.Fatalf("tick did not re-aggregate: %d rows", len(nm.(dashboardModel).rows))
	}
}

func TestDashboardDropsColumnsWhenNarrow(t *testing.T) {
	seedDashHome(t)
	wide := newDashboard(provider.All())
	wm, _ := wide.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	if !strings.Contains(wm.(dashboardModel).View(), "Last used") {
		t.Fatal("wide dashboard should show the Last used column")
	}

	narrow := newDashboard(provider.All())
	nm, _ := narrow.Update(tea.WindowSizeMsg{Width: 46, Height: 34})
	nv := nm.(dashboardModel).View()
	if strings.Contains(nv, "Last used") {
		t.Fatalf("narrow dashboard should drop the Last used column:\n%s", nv)
	}
	// Highest-priority columns must survive the squeeze.
	for _, keep := range []string{"Provider", "Profile", "Identity", "Drift"} {
		if !strings.Contains(nv, keep) {
			t.Fatalf("narrow dashboard dropped priority column %q:\n%s", keep, nv)
		}
	}
	// Also drop the mid-priority Expiry and Dir columns at this width.
	if strings.Contains(nv, "Expiry") {
		t.Fatalf("narrow dashboard should drop the Expiry column:\n%s", nv)
	}
}

func TestDashboardDriftAndExpiryRendering(t *testing.T) {
	future := time.Now().Add(42 * time.Minute)
	m := dashboardModel{width: 120, rows: []dashboardRow{
		{providerName: "azure", providerTitle: "Azure", status: provider.Status{
			ProfileName: "acme", Identity: "u@acme.com", Drifted: true, Expiry: &future, LastUsed: time.Now(),
		}},
	}}
	v := m.View()
	if !strings.Contains(v, "⚠ drift") {
		t.Fatalf("drift marker missing:\n%s", v)
	}
	if !strings.Contains(v, "in ") {
		t.Fatalf("relative expiry missing:\n%s", v)
	}

	past := time.Now().Add(-time.Hour)
	m.rows[0].status.Expiry = &past
	if !strings.Contains(m.View(), "expired") {
		t.Fatalf("expired expiry missing:\n%s", m.View())
	}
}
