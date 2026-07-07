package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// Task 5: "＋ New profile…" is always the PROFILES pane's first row — cursor
// 0 — with real profiles shifted down one. These tests pin the 8 behaviors
// from the task brief.

// Behavior 1: the new-profile row renders first on both a populated and an
// empty tab, and the pane count still reads the real profile count.
func TestNewProfileRowIsAlwaysFirst(t *testing.T) {
	v := twoProfileAwsView(t)
	out := v.View()
	if !strings.Contains(out, "PROFILES (2)") {
		t.Fatalf("profile count should read the real count:\n%s", out)
	}
	idxNew, idxAcme := strings.Index(out, "＋ New profile…"), strings.Index(out, "acme")
	if idxNew < 0 || idxAcme < 0 || idxNew > idxAcme {
		t.Fatalf("new-profile row should render before the first profile row:\n%s", out)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	os.MkdirAll(filepath.Join(home, ".aws-profiles"), 0o755)
	ev := newAwsView()
	nm, _ := ev.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	eout := nm.(awsView).View()
	if !strings.Contains(eout, "PROFILES (0)") || !strings.Contains(eout, "＋ New profile…") {
		t.Fatalf("empty tab should still show the new-profile row and a zero count:\n%s", eout)
	}
}

// Behavior 2: cursor row 0 + enter opens the naming prompt; confirming execs
// the create login (non-nil cmd — the established assertion pattern, see
// TestNewProfilePromptsForNameThenExecsCreate; --no-link is verified by
// reading provider_view.go, there is no argv seam to assert against here).
func TestRowZeroEnterOpensPromptThenCreates(t *testing.T) {
	v := twoProfileAwsView(t)
	if v.cursor != 0 {
		t.Fatalf("expected the new-profile row selected by default, cursor=%d", v.cursor)
	}
	nv, cmd := v.providerTabView.update(tea.KeyMsg{Type: tea.KeyEnter})
	if nv.namingVerb != "login" || cmd != nil {
		t.Fatalf("enter on row 0 should open the create prompt, verb=%q cmd=%v", nv.namingVerb, cmd)
	}
	for _, r := range "fresh" {
		nv, _ = nv.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	nv, cmd = nv.update(tea.KeyMsg{Type: tea.KeyEnter})
	if nv.namingVerb != "" || cmd == nil {
		t.Fatal("enter should close the prompt and exec the create login")
	}
}

// Behavior 3: row 0's DETAILS pane shows the entity blurb and the ACTIONS
// radio lists every persona action, all disabled with "select a profile
// first" — the never-hide model's chosen expression (see task report).
func TestRowZeroDetailsBlurbAndAllActionsDisabled(t *testing.T) {
	v := twoProfileAwsView(t)
	// The DETAILS pane wraps this to fit its column and the frame border sits
	// between the wrapped lines, so compare on whitespace-collapsed text
	// rather than requiring the exact single-line string.
	norm := strings.Join(strings.Fields(strings.ReplaceAll(v.View(), "│", " ")), " ")
	const blurb = "A profile is a container for tokens and intention — sign in once, link it to any number of directories."
	if !strings.Contains(norm, blurb) {
		t.Fatalf("row 0 DETAILS should show the entity blurb:\n%s", v.View())
	}
	acts := v.enabledActions()
	if len(acts) == 0 {
		t.Fatal("row 0 should still list every persona action, just disabled")
	}
	for _, a := range acts {
		if a.enabled {
			t.Fatalf("action %q should be disabled while row 0 is selected", a.key)
		}
		if a.hint != "select a profile first" {
			t.Fatalf("action %q hint = %q, want %q", a.key, a.hint, "select a profile first")
		}
	}
}

// Behavior 4: persona accelerators on row 0 surface the disabled reason and
// dispatch nothing.
func TestRowZeroAcceleratorsAreDisabled(t *testing.T) {
	base := twoProfileAwsView(t).providerTabView
	cases := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("s")},
		{Type: tea.KeyRunes, Runes: []rune("t")},
		{Type: tea.KeyRunes, Runes: []rune("c")},
		{Type: tea.KeyRunes, Runes: []rune("b")},
		{Type: tea.KeyDelete},
	}
	for _, msg := range cases {
		nv, cmd := base.update(msg)
		if cmd != nil {
			t.Fatalf("%q on row 0 must not dispatch, got a command", msg.String())
		}
		if !strings.Contains(nv.status, "select a profile first") {
			t.Fatalf("%q on row 0 should surface the disabled reason, got %q", msg.String(), nv.status)
		}
	}
}

// Behavior 5: the create prompt's confirm hint reads the entity-flavored copy.
func TestCreatePromptHintText(t *testing.T) {
	v := twoProfileAwsView(t)
	nv, _ := v.providerTabView.update(tea.KeyMsg{Type: tea.KeyEnter}) // row 0 -> prompt
	if !strings.Contains(nv.View(), "token container + sign-in — link it later") {
		t.Fatalf("create prompt missing its confirm hint:\n%s", nv.View())
	}
}

// Behavior 6: 'n' still opens the prompt from any row, but New profile is
// gone from the ACTIONS radio (everyday count 5; empty-state count 1 —
// Capture only); the empty-pane copy points at the row instead.
func TestNKeyOpensPromptButHiddenFromRadio(t *testing.T) {
	v := twoProfileAwsView(t)
	nv, _ := v.providerTabView.update(tea.KeyMsg{Type: tea.KeyDown}) // off row 0, proves "from anywhere"
	nv, _ = nv.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if nv.namingVerb != "login" {
		t.Fatalf("'n' should open the create prompt from any row, verb=%q", nv.namingVerb)
	}
	if out := v.View(); !strings.Contains(out, "ACTIONS (5)") {
		t.Fatalf("everyday ACTIONS count should be 5:\n%s", out)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	os.MkdirAll(filepath.Join(home, ".aws-profiles"), 0o755)
	ev := newAwsView()
	nm, _ := ev.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	eout := nm.(awsView).View()
	if !strings.Contains(eout, "ACTIONS (1)") || !strings.Contains(eout, "Capture session") {
		t.Fatalf("empty-state ACTIONS should be Capture only:\n%s", eout)
	}
	if !strings.Contains(eout, "no profiles") || !strings.Contains(eout, "＋ New profile creates") || !strings.Contains(eout, "adopts a live session") {
		t.Fatalf("empty-pane copy should point at the new-profile row:\n%s", eout)
	}
}

// Behavior 7: row 0 gets its own mouse zone; a first click selects it, a
// second click (mirroring clickProfile's reselect semantics) opens the
// prompt. Also covers the reactivated disabled-action click path.
func TestClickNewRowSelectsThenOpensPrompt(t *testing.T) {
	v := twoProfileAwsView(t)
	nv, _ := v.providerTabView.clickProfile("acme") // move off row 0 first
	nv2, cmd := nv.clickNewRow()
	if cmd != nil {
		t.Fatalf("first click on the new-profile row should only select it, got cmd %v", cmd)
	}
	if nv2.cursor != 0 || nv2.focus != focusProfiles {
		t.Fatalf("first click should select row 0: cursor=%d focus=%d", nv2.cursor, nv2.focus)
	}
	nv3, cmd := nv2.clickNewRow()
	if nv3.namingVerb != "login" || cmd != nil {
		t.Fatalf("second click on the new-profile row should open the prompt, verb=%q", nv3.namingVerb)
	}
}

func TestClickDisabledActionAtRowZeroSurfacesReason(t *testing.T) {
	v := twoProfileAwsView(t) // cursor 0 by default
	nv, cmd := v.providerTabView.clickAction("s")
	if cmd != nil {
		t.Fatal("clicking a disabled action must not dispatch")
	}
	if !strings.Contains(nv.status, "select a profile first") {
		t.Fatalf("clicking a disabled action should surface its reason, got %q", nv.status)
	}
}

// TestClickNewProfileRowThroughZones is the real zone-integration path,
// mirroring TestClickProfileRowThroughZones for the new synthetic row.
func TestClickNewProfileRowThroughZones(t *testing.T) {
	m := seedTabs(t)                                                      // seeds azure profile "acme"
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")}) // dashboard -> azure tab
	tm := nm.(tabsModel)
	_ = tm.View()
	time.Sleep(120 * time.Millisecond) // bubblezone records zones asynchronously
	z := zone.Get("prof:+new")
	if z == nil || z.IsZero() {
		t.Skip("new-profile zone not recorded at this size")
	}
	click := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	nm, _ = tm.Update(click)
	av := nm.(tabsModel).tabs[1].model.(azureView)
	if av.cursor != 0 || av.focus != focusProfiles {
		t.Fatalf("first click should select row 0: cursor=%d focus=%d", av.cursor, av.focus)
	}
	nm2, _ := nm.(tabsModel).Update(click)
	av2 := nm2.(tabsModel).tabs[1].model.(azureView)
	if av2.namingVerb != "login" {
		t.Fatalf("second click on row 0 should open the create prompt, verb=%q", av2.namingVerb)
	}
}

// Behavior 8: up from row 0 still hands focus to the tab bar.
func TestUpFromRowZeroHandsFocusToTabs(t *testing.T) {
	v := twoProfileAwsView(t)
	if v.cursor != 0 {
		t.Fatalf("expected row 0 selected by default, got cursor=%d", v.cursor)
	}
	_, cmd := v.providerTabView.update(tea.KeyMsg{Type: tea.KeyUp})
	if cmd == nil {
		t.Fatal("up from row 0 should hand focus to the tab bar")
	}
}
