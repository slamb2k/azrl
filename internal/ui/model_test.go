package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/browserpick"
)

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
	if !strings.Contains(v, "PROFILES (1)") || !strings.Contains(v, "DETAILS") {
		t.Fatalf("view missing pane titles:\n%s", v)
	}
}

func TestHelpBarListsOnlyWiredKeys(t *testing.T) {
	m := seedModel(t)
	help := m.helpBar()
	// keys that are actually wired (labels now sit beside keycap chips)
	for _, k := range []string{"open/run", "back", "quit"} {
		if !strings.Contains(help, k) {
			t.Fatalf("help missing wired key %q: %q", k, help)
		}
	}
}

func TestArrowsMoveFocusBetweenPanes(t *testing.T) {
	m := seedModel(t)
	if m.focus != focusProfiles {
		t.Fatalf("initial focus = %d, want profiles", m.focus)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if nm.(Model).focus != focusActions {
		t.Fatal("right arrow did not move focus to the detail pane")
	}
	nm2, _ := nm.(Model).Update(tea.KeyMsg{Type: tea.KeyLeft})
	if nm2.(Model).focus != focusProfiles {
		t.Fatal("left arrow did not return focus to the profiles pane")
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
	// no label: the slug renders with the tenant detail and the grey ● icon.
	plain := render(item{name: "acme", tenant: "acme.com"})
	if !strings.Contains(plain, "●  acme") || !strings.Contains(plain, "acme.com") {
		t.Fatalf("plain item render:\n%s", plain)
	}
	// only the global default carries 🌐.
	if global := render(item{name: "acme", tenant: "acme.com", scope: scopeGlobal}); !strings.Contains(global, "🌐 acme") {
		t.Fatalf("global-default item missing 🌐:\n%s", global)
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
	m.signedIn = "u@fiig.com.au · fiig.com.au"
	m.ambientWho = "u@fiig.com.au · velrada.com"
	got := m.identityStrip()
	if !strings.Contains(got, ".envrc") {
		t.Fatalf("drift strip should offer .envrc: %q", got)
	}
	// The warning names both tenant-qualified sides (the B2B guest case);
	// the line may word-wrap, so assert the pieces rather than contiguity.
	for _, want := range []string{"velrada.com", "expects u@fiig.com.au", "fiig.com.au"} {
		if !strings.Contains(got, want) {
			t.Fatalf("drift strip missing %q: %q", want, got)
		}
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

func TestKeyHelpFitDropsOptionalTailWhenNarrow(t *testing.T) {
	core := []string{"↑↓", "select", "↵", "run"}
	optional := []string{"q", "quit", "o", "options"}
	wide := keyHelpFit(200, core, optional)
	if !strings.Contains(wide, "options") || !strings.Contains(wide, "quit") {
		t.Fatalf("wide bar should keep optional items: %q", wide)
	}
	narrow := keyHelpFit(30, core, optional)
	if strings.Contains(narrow, "options") {
		t.Fatalf("narrow bar should drop the optional tail: %q", narrow)
	}
	if !strings.Contains(narrow, "select") || !strings.Contains(narrow, "run") {
		t.Fatalf("narrow bar must keep the core: %q", narrow)
	}
	// Optional items drop right-to-left: quit survives longer than options.
	mid := keyHelpFit(60, core, optional)
	if strings.Contains(mid, "options") && !strings.Contains(mid, "quit") {
		t.Fatalf("drop order should favour earlier optional items: %q", mid)
	}
}

func TestAzureBrowserActionOpensPickerAndWritesKeys(t *testing.T) {
	m := seedModel(t)
	confPath := filepath.Join(os.Getenv("HOME"), ".azure-profiles", "acme.conf")
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	mm := nm.(Model)
	if mm.browserFor != "acme" {
		t.Fatalf("'b' did not arm browserFor: %q", mm.browserFor)
	}
	if cmd == nil {
		t.Fatal("expected a discovery command from the browser hotkey")
	}
	nm2, _ := mm.Update(browserProfilesMsg{forProfile: "acme", profiles: []browserpick.Profile{
		{Browser: "edge", OS: "linux", Dir: "Profile 2", Name: "Work", Email: "simon@acme.com"},
	}})
	mm2 := nm2.(Model)
	if mm2.browserPick == nil {
		t.Fatal("browser profiles msg did not open the picker")
	}
	if !strings.Contains(mm2.View(), "BROWSER PROFILE") {
		t.Fatalf("picker not rendered:\n%s", mm2.View())
	}
	nm3, _ := mm2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm3 := nm3.(Model)
	if mm3.browserPick != nil {
		t.Fatal("picker should close after enter")
	}
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), `AZ_BROWSER_CMD=microsoft-edge --profile-directory="Profile 2"`) ||
		!strings.Contains(string(b), "AZ_BROWSER_LABEL=Edge — Work") {
		t.Fatalf("keys not written:\n%s", b)
	}
}

func TestAzureBrowserDiscoveryFailureFallsBackToManualInput(t *testing.T) {
	m := seedModel(t)
	confPath := filepath.Join(os.Getenv("HOME"), ".azure-profiles", "acme.conf")
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	nm2, _ := nm.(Model).Update(browserProfilesMsg{forProfile: "acme", err: os.ErrDeadlineExceeded})
	mm2 := nm2.(Model)
	if !mm2.browserManual {
		t.Fatal("discovery failure did not fall back to manual input")
	}
	mm2.browserInput.SetValue("my-browser --foo")
	nm3, _ := mm2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if nm3.(Model).browserManual {
		t.Fatal("manual input should close after enter")
	}
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), "AZ_BROWSER_CMD=my-browser --foo") {
		t.Fatalf("manual command not written:\n%s", b)
	}
}

func TestAzureBrowserEscClearsStatus(t *testing.T) {
	m := seedModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	mm := nm.(Model)
	if mm.status == "" {
		t.Fatal("expected a status message while discovery is pending")
	}
	// Manual-entry esc.
	nm2, _ := mm.Update(browserProfilesMsg{forProfile: "acme", err: os.ErrDeadlineExceeded})
	mm2 := nm2.(Model)
	nm3, _ := mm2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm3 := nm3.(Model)
	if mm3.status != "" {
		t.Fatalf("esc from manual entry left a stale status: %q", mm3.status)
	}

	// Picker esc.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	mm = nm.(Model)
	nm2, _ = mm.Update(browserProfilesMsg{forProfile: "acme", profiles: []browserpick.Profile{
		{Browser: "edge", OS: "linux", Dir: "Profile 2", Name: "Work", Email: "simon@acme.com"},
	}})
	mm2 = nm2.(Model)
	nm3b, _ := mm2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm3b := nm3b.(Model)
	if mm3b.status != "" {
		t.Fatalf("esc from picker left a stale status: %q", mm3b.status)
	}
}

func TestAzureNewProfilePromptExecsCreateLogin(t *testing.T) {
	m := seedModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	mm := nm.(Model)
	if !mm.creating {
		t.Fatal("'i' did not open the new-profile prompt")
	}
	for _, r := range "fresh" {
		nm, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		mm = nm.(Model)
	}
	nm, cmd := mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = nm.(Model)
	if mm.creating || cmd == nil || !mm.busy {
		t.Fatalf("enter should exec the create login (creating=%v busy=%v)", mm.creating, mm.busy)
	}
}
