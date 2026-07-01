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

func TestTabsRendersBothTabsAzureActiveByDefault(t *testing.T) {
	m := seedTabs(t)
	if m.active != 0 {
		t.Fatalf("default active tab = %d, want 0 (Azure)", m.active)
	}
	v := m.View()
	if !strings.Contains(v, "Azure") || !strings.Contains(v, "GitHub") {
		t.Fatalf("tab bar missing a provider title:\n%s", v)
	}
	// Azure tab active → its banner wordmark is on screen.
	if !strings.Contains(v, "█") {
		t.Fatalf("Azure tab body (banner) not rendered:\n%s", v)
	}
}

func TestTabsSwitchToGitHubAndBack(t *testing.T) {
	m := seedTabs(t)

	// ']' advances to the GitHub tab.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	gh := nm.(tabsModel)
	if gh.active != 1 {
		t.Fatalf("after ']', active = %d, want 1 (GitHub)", gh.active)
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
	if nm2.(tabsModel).active != 0 {
		t.Fatal("'[' did not return to the Azure tab")
	}
}

func TestTabsForwardsKeysToActiveTab(t *testing.T) {
	m := seedTabs(t)
	// On the GitHub tab, 'tab' toggles the inner pane focus (profiles<->actions).
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	gh := nm.(tabsModel)
	before := gh.tabs[1].model.(githubView).focus
	nm2, _ := gh.Update(tea.KeyMsg{Type: tea.KeyTab})
	after := nm2.(tabsModel).tabs[1].model.(githubView).focus
	if before == after {
		t.Fatal("tab key was not forwarded to the active GitHub view")
	}
}
