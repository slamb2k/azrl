package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// seedTabs returns a sized tab container with one Azure and one GitHub profile
// on disk.
func seedTabs(t *testing.T) tabsModel {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	os.MkdirAll(filepath.Join(home, ".github-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".github-profiles", "work.conf"), []byte("GH_HOST=github.com\nGH_USER=octocat\n"), 0o644)

	m := NewTabs()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	return nm.(tabsModel)
}

func TestTabsRendersDashboardActiveByDefault(t *testing.T) {
	m := seedTabs(t)
	if m.active != 0 {
		t.Fatalf("default active tab = %d, want 0 (Dashboard)", m.active)
	}
	v := m.View()
	if !strings.Contains(v, "Dashboard") || !strings.Contains(v, "Azure") || !strings.Contains(v, "GitHub") {
		t.Fatalf("tab bar missing a title:\n%s", v)
	}
	// Dashboard tab active → the seeded profiles from both providers are listed.
	if !strings.Contains(v, "acme") || !strings.Contains(v, "work") {
		t.Fatalf("dashboard body missing aggregated profiles:\n%s", v)
	}
	// Azure banner must NOT show while the dashboard is active.
	if strings.Contains(v, "█") {
		t.Fatalf("Azure banner leaked into the dashboard:\n%s", v)
	}
}

func TestTabsSwitchToGitHubAndBack(t *testing.T) {
	m := seedTabs(t)

	// ']' twice advances dashboard → Azure → GitHub.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	nm, _ = nm.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	gh := nm.(tabsModel)
	if gh.active != 2 {
		t.Fatalf("after ']]', active = %d, want 2 (GitHub)", gh.active)
	}
	v := gh.View()
	if !strings.Contains(v, "PROFILES") || !strings.Contains(v, "work") {
		t.Fatalf("GitHub tab body missing profile list:\n%s", v)
	}
	// Azure banner must NOT show while GitHub is active.
	if strings.Contains(v, "█") {
		t.Fatalf("Azure banner leaked into GitHub tab:\n%s", v)
	}

	// '[' returns to Azure.
	nm2, _ := gh.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if nm2.(tabsModel).active != 1 {
		t.Fatal("'[' did not return to the Azure tab")
	}
}

func TestTabsForwardsKeysToActiveTab(t *testing.T) {
	m := seedTabs(t)
	// Advance to the GitHub tab (index 2): 'tab' toggles inner pane focus.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	nm, _ = nm.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	gh := nm.(tabsModel)
	before := gh.tabs[2].model.(githubView).focus
	nm2, _ := gh.Update(tea.KeyMsg{Type: tea.KeyTab})
	after := nm2.(tabsModel).tabs[2].model.(githubView).focus
	if before == after {
		t.Fatal("tab key was not forwarded to the active GitHub view")
	}
}

func TestTabsSwitchTabMsgSelectsProvider(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(switchTabMsg{provider: "github", profile: "work"})
	if nm.(tabsModel).active != 2 {
		t.Fatalf("switchTabMsg did not select the GitHub tab: active=%d", nm.(tabsModel).active)
	}
}

func TestSwitchTabMsgPreselectsProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	os.WriteFile(filepath.Join(az, "beta.conf"), []byte("AZ_TENANT=beta.com\n"), 0o644)
	gh := filepath.Join(home, ".github-profiles")
	os.MkdirAll(gh, 0o755)
	os.WriteFile(filepath.Join(gh, "play.conf"), []byte("GH_HOST=github.com\n"), 0o644)
	os.WriteFile(filepath.Join(gh, "work.conf"), []byte("GH_HOST=github.com\n"), 0o644)

	m := NewTabs()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	tm := nm.(tabsModel)

	// Jumping to GitHub's "work" (2nd, sorted after "play") moves its cursor there.
	gm, _ := tm.Update(switchTabMsg{provider: "github", profile: "work"})
	gv := gm.(tabsModel).tabs[2].model.(githubView)
	if got := gv.profiles[gv.cursor].Name; got != "work" {
		t.Fatalf("github cursor on %q, want work", got)
	}

	// Jumping to Azure's "beta" (2nd, sorted after "acme") pre-selects it.
	am, _ := tm.Update(switchTabMsg{provider: "azure", profile: "beta"})
	av := am.(tabsModel).tabs[1].model.(Model)
	if sel, _ := av.list.SelectedItem().(item); sel.name != "beta" {
		t.Fatalf("azure cursor on %q, want beta", sel.name)
	}
}

func TestProviderTabsIgnoreDashboardMessages(t *testing.T) {
	seedTabs(t)
	// The provider views must ignore dashboard-only messages without panicking.
	az, _ := NewModel().Update(dashboardTickMsg{})
	if _, ok := az.(Model); !ok {
		t.Fatal("Azure Model did not survive dashboardTickMsg")
	}
	az2, _ := NewModel().Update(switchTabMsg{provider: "azure"})
	if _, ok := az2.(Model); !ok {
		t.Fatal("Azure Model did not survive switchTabMsg")
	}
	gh, _ := newGithubView().Update(dashboardTickMsg{})
	if _, ok := gh.(githubView); !ok {
		t.Fatal("githubView did not survive dashboardTickMsg")
	}
}

func TestNewTabsForProviderSelectsNamedTab(t *testing.T) {
	seedTabs(t)
	m := NewTabsForProvider("github")
	if m.tabs[m.active].name != "github" {
		t.Fatalf("NewTabsForProvider(github) active tab = %q", m.tabs[m.active].name)
	}
}
