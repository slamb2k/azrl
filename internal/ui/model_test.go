package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestContextLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)

	// dir linked via .azprofile
	linked := t.TempDir()
	os.WriteFile(filepath.Join(linked, ".azprofile"), []byte("acme\n"), 0o644)
	if got := contextLine(linked); !strings.Contains(got, "This dir") || !strings.Contains(got, "acme") {
		t.Fatalf("linked: %q", got)
	}

	// dir whose basename matches an existing conf -> offer link
	matchDir := filepath.Join(t.TempDir(), "acme")
	os.MkdirAll(matchDir, 0o755)
	if got := contextLine(matchDir); !strings.Contains(strings.ToLower(got), "link") {
		t.Fatalf("match: %q", got)
	}

	// unknown dir -> offer create
	if got := contextLine(filepath.Join(t.TempDir(), "brand-new")); !strings.Contains(strings.ToLower(got), "create") {
		t.Fatalf("unknown: %q", got)
	}
}

// seedModel returns a sized model with one profile on disk.
func seedModel(t *testing.T) Model {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	m := NewModel()
	m.width, m.height = 90, 30
	m.layout()
	return m
}

func TestModelViewRenders(t *testing.T) {
	m := seedModel(t)
	v := m.View()
	// The banner now lives in the tab container, not the Azure view.
	if strings.Contains(v, "█") {
		t.Fatalf("Azure view should no longer render the banner wordmark:\n%s", v)
	}
	// every action verb has a home in the action pane
	for _, label := range []string{"Sign in", "Use here", "Capture", "New profile", "Edit", "Remove"} {
		if !strings.Contains(v, label) {
			t.Fatalf("view missing action %q:\n%s", label, v)
		}
	}
	if !strings.Contains(v, "PROFILES (1)") || !strings.Contains(v, "ACTION") {
		t.Fatalf("view missing pane titles:\n%s", v)
	}
}

func TestHelpBarListsOnlyWiredKeys(t *testing.T) {
	m := seedModel(t)
	help := m.helpBar()
	// keys that are actually wired
	for _, k := range []string{"open/run", "esc back", "quit"} {
		if !strings.Contains(help, k) {
			t.Fatalf("help missing wired key %q: %q", k, help)
		}
	}
}

func TestTabTogglesFocus(t *testing.T) {
	m := seedModel(t)
	if m.focus != focusProfiles {
		t.Fatalf("initial focus = %d, want profiles", m.focus)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if nm.(Model).focus != focusActions {
		t.Fatal("tab did not move focus to actions")
	}
}

func TestActionHotkeySelectsRadio(t *testing.T) {
	m := seedModel(t)
	// 'c' selects the Capture action (handoff verb); dispatch returns a cmd.
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if got := nm.(Model).actions.selected().key; got != "c" {
		t.Fatalf("hotkey c selected %q", got)
	}
	if cmd == nil {
		t.Fatal("expected a command from the capture hotkey")
	}
}

func TestEditHotkeySelectsRadio(t *testing.T) {
	m := seedModel(t)
	// 'x' selects the Edit action against the selected profile and returns a cmd.
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if got := nm.(Model).actions.selected().key; got != "x" {
		t.Fatalf("hotkey x selected %q", got)
	}
	if cmd == nil {
		t.Fatal("expected a command from the edit hotkey")
	}
	if !nm.(Model).busy {
		t.Fatal("edit hotkey should mark the model busy")
	}
}

func TestProfileDelegateRendersLabelScopeAndTenant(t *testing.T) {
	render := func(it item) string {
		l := list.New([]list.Item{it}, profileDelegate{}, 40, 10)
		var b strings.Builder
		profileDelegate{}.Render(&b, l, 0, it)
		return b.String()
	}
	// no label: the slug renders with the tenant detail and the default 🌐.
	plain := render(item{name: "acme", tenant: "acme.com"})
	if !strings.Contains(plain, "🌐 acme") || !strings.Contains(plain, "acme.com") {
		t.Fatalf("plain item render:\n%s", plain)
	}
	// with a label: the label renders instead of the slug.
	labeled := render(item{name: "acme", label: "Acme Production", tenant: "contoso.com"})
	if !strings.Contains(labeled, "Acme Production") || !strings.Contains(labeled, "contoso.com") {
		t.Fatalf("labeled item render:\n%s", labeled)
	}
	// The active-identity icon leads the name.
	if pinned := render(item{name: "acme", tenant: "acme.com", scope: ScopeCwd}); !strings.Contains(pinned, "●  acme") {
		t.Fatalf("cwd-pinned item missing leading dot:\n%s", pinned)
	}
	// filtering matches on both slug and label.
	fv := item{name: "acme", label: "Acme Production"}.FilterValue()
	if !strings.Contains(fv, "acme") || !strings.Contains(fv, "Production") {
		t.Fatalf("filter value: %q", fv)
	}
}

func TestRenameEntersInputState(t *testing.T) {
	m := seedModel(t)
	// 'n' opens the rename input seeded with the selected profile's name.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	mm := nm.(Model)
	if !mm.renaming {
		t.Fatal("rename hotkey did not enter the rename state")
	}
	if mm.renameOld != "acme" || mm.rename.Value() != "acme" {
		t.Fatalf("rename input not seeded: old=%q value=%q", mm.renameOld, mm.rename.Value())
	}
	if !strings.Contains(mm.View(), "RENAME") {
		t.Fatal("rename view should show the RENAME prompt")
	}
	// esc cancels without renaming.
	nm2, _ := mm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if nm2.(Model).renaming {
		t.Fatal("esc should cancel the rename")
	}
}

func TestRemoveEntersConfirm(t *testing.T) {
	m := seedModel(t)
	// delete on the selected profile arms the confirm sub-state, does not delete.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	mm := nm.(Model)
	if !mm.confirming {
		t.Fatal("remove hotkey did not enter confirm state")
	}
	if mm.pendingDelete != "acme" {
		t.Fatalf("pendingDelete = %q", mm.pendingDelete)
	}
	if !strings.Contains(mm.View(), "acme") {
		t.Fatal("confirm view should name the profile")
	}
	// 'n' cancels without deleting.
	nm2, _ := mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if nm2.(Model).confirming {
		t.Fatal("'n' should cancel the confirm")
	}
}

func TestDriftHintMentionsEnvrc(t *testing.T) {
	m := seedModel(t)
	m.drift = true
	if got := m.identityStrip(); !strings.Contains(got, ".envrc") || !strings.Contains(got, "press e") {
		t.Fatalf("drift strip should offer .envrc via e: %q", got)
	}
	// without drift, no warning
	m.drift = false
	if strings.Contains(m.identityStrip(), ".envrc") {
		t.Fatal("no drift should not mention .envrc")
	}
}

func TestEnvrcHotkeyNeedsProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	clean := t.TempDir() // no .azprofile anywhere up the tree
	old, _ := os.Getwd()
	os.Chdir(clean)
	defer os.Chdir(old)

	m := NewModel()
	m.width, m.height = 90, 30
	m.layout()
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if cmd != nil {
		t.Fatal("e without a resolved profile should not run a command")
	}
	if !strings.Contains(nm.(Model).status, "no profile") {
		t.Fatalf("expected a 'no profile' status, got %q", nm.(Model).status)
	}
}

func TestHandoffArgs(t *testing.T) {
	cases := []struct {
		key, prof string
		want      []string
	}{
		{"l", "acme", []string{"login", "acme"}},
		{"l", "", []string{"login"}},
		{"i", "acme", []string{"login"}},
		{"c", "acme", []string{"capture"}},
		{"u", "acme", nil},
	}
	for _, c := range cases {
		got := handoffArgs(c.key, c.prof)
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Fatalf("handoffArgs(%q,%q) = %v, want %v", c.key, c.prof, got, c.want)
		}
	}
}

func TestEnterOnProfilesOpensActionPane(t *testing.T) {
	m := seedModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if nm.(Model).focus != focusActions {
		t.Fatal("enter on the profile pane did not open the action pane")
	}
	nm2, _ := nm.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if nm2.(Model).focus != focusProfiles {
		t.Fatal("esc did not return focus to the profile pane")
	}
}

func TestF5Refreshes(t *testing.T) {
	m := seedModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyF5})
	if len(nm.(Model).list.Items()) == 0 {
		t.Fatal("F5 refresh dropped the profile list")
	}
}
