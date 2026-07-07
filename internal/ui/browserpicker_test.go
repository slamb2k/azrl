package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/slamb2k/azrl/internal/browserpick"
)

func seedAwsHome(t *testing.T, confBody string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	confPath := filepath.Join(ap, "work.conf")
	os.WriteFile(confPath, []byte(confBody), 0o644)
	return confPath
}

func TestBrowserActionListedOnProviderTabs(t *testing.T) {
	seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if out := nm.(awsView).View(); !strings.Contains(out, "Assign browser…") {
		t.Fatalf("missing Assign browser… action:\n%s", out)
	}
}

func TestBrowserProfilesMsgOpensPickerAndEnterWritesKeys(t *testing.T) {
	confPath := seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	av := nm.(awsView)
	av.providerTabView.browserFor = "work"
	msg := browserProfilesMsg{forProfile: "work", profiles: []browserpick.Profile{
		{Browser: "edge", OS: "linux", Dir: "Profile 2", Name: "Work", Email: "simon@acme.com"},
	}}
	nm2, _ := av.Update(msg)
	out := nm2.(awsView).View()
	if !strings.Contains(out, "BROWSER PROFILE") || !strings.Contains(out, "Edge — Work") {
		t.Fatalf("picker not rendered:\n%s", out)
	}
	nm3, _ := nm2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), `AWS_BROWSER_CMD=microsoft-edge --profile-directory="Profile 2"`) ||
		!strings.Contains(string(b), "AWS_BROWSER_LABEL=Edge — Work") {
		t.Fatalf("keys not written:\n%s", b)
	}
	if strings.Contains(nm3.(awsView).View(), "BROWSER PROFILE") {
		t.Fatal("picker should close after enter")
	}
}

func TestBrowserDiscoveryFailureFallsBackToManualInput(t *testing.T) {
	confPath := seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	av := nm.(awsView)
	av.providerTabView.browserFor = "work"
	nm2, _ := av.Update(browserProfilesMsg{forProfile: "work", err: os.ErrDeadlineExceeded})
	out := nm2.(awsView).View()
	if !strings.Contains(out, "Browser command") {
		t.Fatalf("manual fallback prompt not shown:\n%s", out)
	}
	av2 := nm2.(awsView)
	av2.providerTabView.browserInput.SetValue("my-browser --foo")
	nm3, _ := av2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = nm3
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), "AWS_BROWSER_CMD=my-browser --foo") {
		t.Fatalf("manual command not written:\n%s", b)
	}
}

func TestDetailsShowsBrowserLabel(t *testing.T) {
	seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_BROWSER_LABEL=Edge — Work\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nm, _ = nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyDown}) // off row 0 (＋ New profile…), onto the profile
	if out := nm.(awsView).View(); !strings.Contains(out, "Edge — Work") {
		t.Fatalf("DETAILS missing browser label:\n%s", out)
	}
}

// twoMatchBrowserPicker opens an AWS view with the browser picker armed with
// two discovered profiles ("Other" at row 0, "Work" at row 1).
func twoMatchBrowserPicker(t *testing.T) (awsView, string) {
	t.Helper()
	confPath := seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	av := nm.(awsView)
	av.providerTabView.browserFor = "work"
	msg := browserProfilesMsg{forProfile: "work", profiles: []browserpick.Profile{
		{Browser: "edge", OS: "linux", Dir: "Profile 1", Name: "Other", Email: "other@acme.com"},
		{Browser: "edge", OS: "linux", Dir: "Profile 2", Name: "Work", Email: "simon@acme.com"},
	}}
	nm2, _ := av.Update(msg)
	return nm2.(awsView), confPath
}

// TestClickOutsideBrowserPickerDismisses is the zone-integration proof for
// the browser picker overlay's outside-click esc path.
func TestClickOutsideBrowserPickerDismisses(t *testing.T) {
	av, _ := twoMatchBrowserPicker(t)
	_ = zone.Scan(av.View())
	time.Sleep(120 * time.Millisecond)
	z := zone.Get("box:browser")
	if z == nil || z.IsZero() {
		t.Fatal("browser box zone missing")
	}
	outside := tea.MouseMsg{X: z.StartX - 1, Y: z.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	nm, _ := av.Update(outside)
	if nm.(awsView).providerTabView.browserPick != nil {
		t.Fatal("click outside the browser picker popup should dismiss it")
	}
}

// TestBrowserPickerClickRowThroughZonesMapsProfile drives a full row click
// through real zone bounds: select, then click-again runs enter (maps the
// profile and closes), proving the bp:<i> wiring end to end.
func TestBrowserPickerClickRowThroughZonesMapsProfile(t *testing.T) {
	av, confPath := twoMatchBrowserPicker(t)
	_ = zone.Scan(av.View())
	time.Sleep(120 * time.Millisecond)
	z := zone.Get("bp:1")
	if z == nil || z.IsZero() {
		t.Fatal("bp:1 row zone missing")
	}
	click := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	nm, _ := av.Update(click)
	av2 := nm.(awsView)
	if av2.providerTabView.browserPick == nil || av2.providerTabView.browserPick.cursor != 1 {
		t.Fatal("first click should select row 1 without closing")
	}
	nm2, _ := av2.Update(click)
	if nm2.(awsView).providerTabView.browserPick != nil {
		t.Fatal("second click on the selected row should map and close the picker")
	}
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), `AWS_BROWSER_CMD=microsoft-edge --profile-directory="Profile 2"`) {
		t.Fatalf("keys not written:\n%s", b)
	}
}

// TestBrowserPickerWheelMovesCursor proves wheel events move the browser
// picker's own cursor while it's open.
func TestBrowserPickerWheelMovesCursor(t *testing.T) {
	av, _ := twoMatchBrowserPicker(t)
	if av.providerTabView.browserPick.cursor != 0 {
		t.Fatalf("expected cursor 0 to start, got %d", av.providerTabView.browserPick.cursor)
	}
	nm, _ := av.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	av2 := nm.(awsView)
	if av2.providerTabView.browserPick.cursor != 1 {
		t.Fatalf("wheel down should move the browser picker cursor, got %d", av2.providerTabView.browserPick.cursor)
	}
	nm2, _ := av2.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if nm2.(awsView).providerTabView.browserPick.cursor != 0 {
		t.Fatalf("wheel up should move back, got %d", nm2.(awsView).providerTabView.browserPick.cursor)
	}
}
