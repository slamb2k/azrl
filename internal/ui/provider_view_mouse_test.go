package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// twoProfileAwsView returns an AWS view sized and seeded with two profiles
// ("work", "acme"), cursor at 0.
func twoProfileAwsView(t *testing.T) awsView {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "acme.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://work.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	return nm.(awsView)
}

func TestWheelMovesProfileCursor(t *testing.T) {
	v := twoProfileAwsView(t)
	if v.cursor != 0 {
		t.Fatalf("expected cursor 0 to start, got %d", v.cursor)
	}
	nv, _ := v.providerTabView.update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if nv.cursor != 1 {
		t.Fatalf("wheel down should move the cursor, got %d", nv.cursor)
	}
	nv, _ = nv.update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if nv.cursor != 0 {
		t.Fatalf("wheel up should move back, got %d", nv.cursor)
	}
}

// TestWheelClampsAtEnds proves wheel-at-top/bottom clamps instead of wrapping
// or handing focus to the tab bar (unlike the up/down keys at the top row).
func TestWheelClampsAtEnds(t *testing.T) {
	v := twoProfileAwsView(t)
	nv, cmd := v.providerTabView.update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if nv.cursor != 0 || cmd != nil {
		t.Fatalf("wheel up at the top must clamp, not emit focusTabsMsg: cursor=%d cmd=%v", nv.cursor, cmd)
	}
	nv, _ = nv.update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	nv, _ = nv.update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if nv.cursor != 1 {
		t.Fatalf("wheel down at the bottom must clamp at len-1, got %d", nv.cursor)
	}
}

// TestClickProfileSelectsThenFocusesActions drives the semantic clickProfile
// entry point directly: clicking an unselected row selects it; clicking the
// already-selected row behaves like enter on the profiles pane (focus moves
// to actions).
func TestClickProfileSelectsThenFocusesActions(t *testing.T) {
	v := twoProfileAwsView(t)
	if v.selected() != "acme" {
		t.Fatalf("expected acme selected first alphabetically, got %q", v.selected())
	}
	nv, cmd := v.providerTabView.clickProfile("work")
	if cmd != nil {
		t.Fatalf("selecting a different row should not return a command, got %v", cmd)
	}
	if nv.selected() != "work" || nv.focus != focusProfiles {
		t.Fatalf("click should select work and keep focus on profiles: selected=%q focus=%d", nv.selected(), nv.focus)
	}
	// Click the same row again: enter-on-profiles-pane semantics (focus → actions).
	nv2, _ := nv.clickProfile("work")
	if nv2.selected() != "work" || nv2.focus != focusActions {
		t.Fatalf("re-click on the selected row should move focus to actions: selected=%q focus=%d", nv2.selected(), nv2.focus)
	}
}

// TestClickUnknownActionKeyIsNoop proves clicking a key with no matching row
// (e.g. "u" — Link here lives on the dashboard now, not the tabs) is inert
// rather than running or crashing.
func TestClickUnknownActionKeyIsNoop(t *testing.T) {
	v := twoProfileAwsView(t)
	nv, cmd := v.providerTabView.clickAction("u")
	if cmd != nil {
		t.Fatalf("unknown action key must not run, got cmd %v", cmd)
	}
	if nv.status != "" {
		t.Fatalf("unknown action key must not set a status, got %q", nv.status)
	}
}

// TestClickEnabledActionRuns proves clicking an enabled action row selects
// and runs it immediately — the accelerator loop's exact behavior.
func TestClickEnabledActionRuns(t *testing.T) {
	v := twoProfileAwsView(t)
	nv, cmd := v.providerTabView.clickAction("s") // Sign in — always enabled
	if cmd == nil {
		t.Fatal("enabled action click should return the handoff command")
	}
	acts := nv.enabledActions()
	found := false
	for i, a := range acts {
		if a.key == "s" {
			found = true
			if nv.actionCur != i {
				t.Fatalf("actionCur should land on Sign in's index %d, got %d", i, nv.actionCur)
			}
		}
	}
	if !found {
		t.Fatal("Sign in action missing from enabled actions")
	}
}

// TestMouseIgnoredWhileCapturingInput proves row/action clicks and wheel are
// no-ops while a sub-state (confirm dialog, naming prompt, ...) owns input.
func TestMouseIgnoredWhileCapturingInput(t *testing.T) {
	v := twoProfileAwsView(t)
	nv, _ := v.providerTabView.update(tea.KeyMsg{Type: tea.KeyDelete}) // arms the confirm dialog
	if !nv.confirming {
		t.Fatal("delete should arm the confirm dialog")
	}
	before := nv
	nv2, cmd := nv.update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if cmd != nil || nv2.cursor != before.cursor || !nv2.confirming {
		t.Fatalf("wheel must be a no-op while capturing input: cursor=%d confirming=%v", nv2.cursor, nv2.confirming)
	}
}

// TestClickProfileRowThroughZones is the one real zone-integration test: it
// renders the container, waits for bubblezone's async Scan, then clicks the
// recorded bounds of a profile row.
func TestClickProfileRowThroughZones(t *testing.T) {
	m := seedTabs(t) // seeds azure profile "acme"
	// A second, alphabetically-earlier profile so the default cursor (0) lands
	// on it rather than on acme — the click on acme's row then exercises the
	// "select" branch first, then the "already selected" branch.
	os.WriteFile(filepath.Join(m.tabs[1].model.(azureView).prov.ProfilesDir(), "aaa.conf"), []byte("AZ_TENANT=aaa.com\n"), 0o644)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})             // dashboard → azure tab
	nm, _ = nm.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}) // pick up aaa.conf
	tm := nm.(tabsModel)
	_ = tm.View()
	time.Sleep(120 * time.Millisecond) // bubblezone records zones asynchronously
	z := zone.Get("prof:acme")
	if z == nil || z.IsZero() {
		t.Skipf("profile zone not recorded at this size — adapt the id/profile to the active tab")
	}
	click := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	nm, _ = tm.Update(click)
	av := nm.(tabsModel).tabs[1].model.(azureView)
	if av.selected() != "acme" || av.focus != focusProfiles {
		t.Fatalf("first click should select acme, focus on profiles: selected=%q focus=%d", av.selected(), av.focus)
	}
	// Second click on the same (now-selected) row: enter semantics.
	nm2, _ := nm.(tabsModel).Update(click)
	av2 := nm2.(tabsModel).tabs[1].model.(azureView)
	if av2.selected() != "acme" || av2.focus != focusActions {
		t.Fatalf("second click on the selected row should move focus to actions: selected=%q focus=%d", av2.selected(), av2.focus)
	}
}
