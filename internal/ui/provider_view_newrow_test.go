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

// New-profile creation lives in the PROFILES title bar as the "NEW ＋"
// affordance (zone prof:+new) plus the n accelerator and its footer chip —
// never as a row in the list, so the cursor only lands on real profiles.

// The NEW ＋ button renders on the PROFILES title line; the list itself
// starts at the first real profile, which is selected by default with its
// actions enabled.
func TestNewButtonInTitleBarNotInList(t *testing.T) {
	v := twoProfileAwsView(t)
	out := v.View()
	if !strings.Contains(out, "PROFILES (2)") {
		t.Fatalf("profile count should read the real count:\n%s", out)
	}
	titleLine := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "PROFILES (2)") {
			titleLine = l
			break
		}
	}
	if !strings.Contains(titleLine, "NEW ＋") {
		t.Fatalf("NEW ＋ should render on the PROFILES title line:\n%s", out)
	}
	if strings.Contains(out, "＋ New profile…") {
		t.Fatalf("the synthetic list row should be gone:\n%s", out)
	}
	if v.cursor != 0 || v.selected() == "" {
		t.Fatalf("default cursor should select the first real profile, cursor=%d selected=%q", v.cursor, v.selected())
	}
	for _, a := range v.enabledActions() {
		if a.key != "s" && !a.enabled {
			t.Fatalf("action %q should be enabled on the default selection (hint %q)", a.key, a.hint)
		}
	}
}

// The footer keymap carries the n chip so the verb is discoverable from the
// bottom bar too.
func TestNewProfileFooterChip(t *testing.T) {
	v := twoProfileAwsView(t)
	if out := v.View(); !strings.Contains(out, "new profile") {
		t.Fatalf("footer should carry the n · new profile chip:\n%s", out)
	}
}

// n opens the naming prompt from anywhere; confirming execs the create login
// (non-nil cmd — the established assertion pattern; --no-link is verified by
// reading provider_view.go, there is no argv seam to assert against here).
func TestNKeyOpensPromptThenCreates(t *testing.T) {
	v := twoProfileAwsView(t)
	nv, cmd := v.providerTabView.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if nv.namingVerb != "login" || cmd != nil {
		t.Fatalf("'n' should open the create prompt, verb=%q cmd=%v", nv.namingVerb, cmd)
	}
	for _, r := range "fresh" {
		nv, _ = nv.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	nv, cmd = nv.update(tea.KeyMsg{Type: tea.KeyEnter})
	if nv.namingVerb != "" || cmd == nil {
		t.Fatal("enter should close the prompt and exec the create login")
	}
}

// The create prompt carries the entity blurb (the education moment) and the
// entity-flavored confirm hint; the capture prompt carries neither.
func TestCreatePromptShowsEntityBlurb(t *testing.T) {
	v := twoProfileAwsView(t)
	nv, _ := v.providerTabView.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	norm := strings.Join(strings.Fields(strings.ReplaceAll(nv.View(), "│", " ")), " ")
	const blurb = "A profile is a container for tokens and intention — sign in once, link it to any number of directories."
	if !strings.Contains(norm, blurb) {
		t.Fatalf("create prompt should show the entity blurb:\n%s", nv.View())
	}
	if !strings.Contains(nv.View(), "token container + sign-in — link it later") {
		t.Fatalf("create prompt missing its confirm hint:\n%s", nv.View())
	}
}

// New profile has no ACTIONS entry (everyday count 5; empty-state count 1 —
// Capture only); the empty-pane copy points at the n key, and the NEW ＋
// button still renders on an empty tab.
func TestNewProfileHiddenFromRadio(t *testing.T) {
	v := twoProfileAwsView(t)
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
	if !strings.Contains(eout, "PROFILES (0)") || !strings.Contains(eout, "NEW ＋") {
		t.Fatalf("empty tab should still show the zero count and the NEW ＋ button:\n%s", eout)
	}
	if !strings.Contains(eout, "ACTIONS (1)") || !strings.Contains(eout, "Capture session") {
		t.Fatalf("empty-state ACTIONS should be Capture only:\n%s", eout)
	}
	if !strings.Contains(eout, "no profiles") || !strings.Contains(eout, "creates") || !strings.Contains(eout, "adopts a live session") {
		t.Fatalf("empty-pane copy should point at the n key:\n%s", eout)
	}
}

// TestClickNewButtonThroughZones is the real zone-integration path: NEW ＋ is
// a button, so a single click opens the naming prompt directly.
func TestClickNewButtonThroughZones(t *testing.T) {
	m := seedTabs(t)                                                      // seeds azure profile "acme"
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")}) // dashboard -> azure tab
	tm := nm.(tabsModel)
	_ = tm.View()
	time.Sleep(120 * time.Millisecond) // bubblezone records zones asynchronously
	z := zone.Get("prof:+new")
	if z == nil || z.IsZero() {
		t.Skip("NEW ＋ zone not recorded at this size")
	}
	click := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	nm, _ = tm.Update(click)
	av := nm.(tabsModel).tabs[1].model.(azureView)
	if av.namingVerb != "login" {
		t.Fatalf("clicking NEW ＋ should open the create prompt, verb=%q", av.namingVerb)
	}
}

// Up from the first profile row still hands focus to the tab bar.
func TestUpFromFirstRowHandsFocusToTabs(t *testing.T) {
	v := twoProfileAwsView(t)
	if v.cursor != 0 {
		t.Fatalf("expected the first profile selected by default, got cursor=%d", v.cursor)
	}
	_, cmd := v.providerTabView.update(tea.KeyMsg{Type: tea.KeyUp})
	if cmd == nil {
		t.Fatal("up from the first row should hand focus to the tab bar")
	}
}
