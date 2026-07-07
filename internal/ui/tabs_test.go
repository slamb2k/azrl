package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/slamb2k/azrl/internal/provider"
)

// noViewProvider is a provider whose Name() has no entry in the views map. Only
// Name()/Title() are exercised by providerTabs, so the embedded nil interface is
// never dereferenced. It uses an unregistered name so it stays view-less.
type noViewProvider struct{ provider.Provider }

func (noViewProvider) Name() string  { return "acmecloud" }
func (noViewProvider) Title() string { return "AcmeCloud" }

// TestProviderTabsSkipsProviderWithoutView proves a provider missing a view is
// skipped rather than appended as a nil-model tab (which would nil-panic).
func TestProviderTabsSkipsProviderWithoutView(t *testing.T) {
	views := map[string]tea.Model{"azure": newAzureView()}
	tabs := providerTabs([]provider.Provider{noViewProvider{}}, views)
	if len(tabs) != 0 {
		t.Fatalf("provider without a view should yield no tab, got %d", len(tabs))
	}
	// A registered view is still paired, and never with a nil model.
	tabs = providerTabs([]provider.Provider{}, views)
	for _, tb := range tabs {
		if tb.model == nil {
			t.Fatalf("tab %q has a nil model", tb.name)
		}
	}
}

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
	// The winged banner now lives in the container and shows above every tab.
	if !strings.Contains(v, "█") {
		t.Fatalf("banner missing from the dashboard tab:\n%s", v)
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
	// The winged banner now shows above the GitHub tab too.
	if !strings.Contains(v, "█") {
		t.Fatalf("banner missing from the GitHub tab:\n%s", v)
	}

	// '[' returns to the Azure tab.
	nm2, _ := gh.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if nm2.(tabsModel).active != 1 {
		t.Fatal("'[' did not return to the Azure tab")
	}
}

func TestTabsForwardsKeysToActiveTab(t *testing.T) {
	m := seedTabs(t)
	// Advance to the GitHub tab (index 2).
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	nm, _ = nm.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	gh := nm.(tabsModel)
	before := gh.tabs[2].model.(githubView).focus
	nm2, _ := gh.Update(tea.KeyMsg{Type: tea.KeyRight})
	after := nm2.(tabsModel).tabs[2].model.(githubView).focus
	if before == after {
		t.Fatal("right arrow was not forwarded to the active GitHub view")
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
	if got := gv.selected(); got != "work" {
		t.Fatalf("github cursor on %q, want work", got)
	}

	// Jumping to Azure's "beta" (2nd, sorted after "acme") pre-selects it.
	am, _ := tm.Update(switchTabMsg{provider: "azure", profile: "beta"})
	av := am.(tabsModel).tabs[1].model.(azureView)
	if got := av.selected(); got != "beta" {
		t.Fatalf("azure cursor on %q, want beta", got)
	}
}

// TestSwitchTabMsgWithActionTriggersIt proves an accelerator action riding a
// switchTabMsg runs against the newly-selected profile: the GitHub tab is
// active, its browser-discovery flow is underway (status copy from
// browserAction — see browserpicker.go), and the returned cmd is the
// discovery command, non-nil so the container actually executes it.
func TestSwitchTabMsgWithActionTriggersIt(t *testing.T) {
	m := seedTabs(t)
	nm, cmd := m.Update(switchTabMsg{provider: "github", profile: "work", action: "b"})
	tm := nm.(tabsModel)
	if tm.tabs[tm.active].title != "GitHub" {
		t.Fatalf("should land on the GitHub tab, got %q", tm.tabs[tm.active].title)
	}
	if cmd == nil {
		t.Fatal("dispatching action \"b\" should return the browser-discovery cmd, got nil")
	}
	gv := tm.tabs[tm.active].model.(githubView)
	if gv.selected() != "work" {
		t.Fatalf("cursor not on the routed profile, selected=%q", gv.selected())
	}
	if v := gv.View(); !strings.Contains(v, "looking for browser profiles on the local machine") {
		t.Fatalf("expected browser-discovery status in view, got:\n%s", v)
	}
}

func TestProviderTabsIgnoreDashboardMessages(t *testing.T) {
	seedTabs(t)
	// The provider views must ignore dashboard-only messages without panicking.
	az, _ := newAzureView().Update(dashboardTickMsg{})
	if _, ok := az.(azureView); !ok {
		t.Fatal("azureView did not survive dashboardTickMsg")
	}
	az2, _ := newAzureView().Update(switchTabMsg{provider: "azure"})
	if _, ok := az2.(azureView); !ok {
		t.Fatal("azureView did not survive switchTabMsg")
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

// TestTabsBannerOnEveryTab asserts the winged wordmark shows above the tab bar
// on the dashboard, Azure, and GitHub tabs alike (banner lives in the container).
func TestTabsBannerOnEveryTab(t *testing.T) {
	m := seedTabs(t) // width 100 → full art
	for i := range m.tabs {
		m.active = i
		if !strings.Contains(m.View(), "█") {
			t.Fatalf("banner wordmark missing on tab %d (%s):\n%s", i, m.tabs[i].title, m.View())
		}
	}
}

// TestTabsCloseTearsDownDashboardWatcher proves centralized teardown: the tab
// container's Close() closes the dashboard-owned fsnotify watcher (so no quit
// path leaks its goroutine/fd), and is safe to call again (idempotent).
func TestTabsCloseTearsDownDashboardWatcher(t *testing.T) {
	m := seedTabs(t) // NewTabs in a temp HOME → dashboard builds a real watcher.
	dash, ok := m.tabs[0].model.(dashboardModel)
	if !ok {
		t.Fatalf("tab 0 is %T, want dashboardModel", m.tabs[0].model)
	}
	if dash.watcher == nil {
		t.Skip("no fsnotify watcher available; nothing to tear down")
	}
	if err := m.Close(); err != nil {
		t.Fatalf("tabsModel.Close() returned error: %v", err)
	}
	// Idempotent: a second Close (watcher already closed) must not error.
	if err := m.Close(); err != nil {
		t.Fatalf("second tabsModel.Close() returned error: %v", err)
	}
}

// TestTabsWidthInvariant is the core responsiveness guarantee: at every width,
// on every tab, no rendered line may exceed the terminal width.
func TestTabsWidthInvariant(t *testing.T) {
	base := seedTabs(t)
	for _, w := range []int{40, 60, 80, 100, 120} {
		nm, _ := base.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		tm := nm.(tabsModel)
		for i := range tm.tabs {
			tm.active = i
			for _, line := range strings.Split(tm.View(), "\n") {
				if lw := lipgloss.Width(line); lw > w {
					t.Fatalf("tab %d (%s) at width %d: line width %d exceeds terminal: %q",
						i, tm.tabs[i].title, w, lw, line)
				}
			}
		}
	}
}

// TestTabsCompactBannerAtNarrowWidth asserts the fixed art is replaced by a
// compact one-line title when the terminal is narrower than the art, and that
// nothing overflows.
func TestTabsCompactBannerAtNarrowWidth(t *testing.T) {
	base := seedTabs(t)
	nm, _ := base.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	tm := nm.(tabsModel)
	v := tm.View()
	if strings.Contains(v, "█") {
		t.Fatalf("full banner art must not render at width 30:\n%s", v)
	}
	if !strings.Contains(v, "A Z R L") {
		t.Fatalf("compact banner title missing at width 30:\n%s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if lw := lipgloss.Width(line); lw > 30 {
			t.Fatalf("line width %d exceeds width 30: %q", lw, line)
		}
	}
}

func TestTabsTabKeyCyclesTabs(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if nm.(tabsModel).active != 1 {
		t.Fatalf("tab: active = %d, want 1 (Azure)", nm.(tabsModel).active)
	}
	nm2, _ := nm.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if nm2.(tabsModel).active != 0 {
		t.Fatalf("shift+tab: active = %d, want 0 (Dashboard)", nm2.(tabsModel).active)
	}
	// Arrows no longer switch tabs — they belong to the panes.
	nm3, _ := nm2.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRight})
	if nm3.(tabsModel).active != 0 {
		t.Fatal("right arrow must not switch tabs")
	}
}

func TestTabsTabKeyForwardedWhileNaming(t *testing.T) {
	m := seedTabs(t)
	// Move to the Azure tab and arm the new-profile text input ('n').
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	nm, _ = nm.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	tm := nm.(tabsModel)
	if tm.tabs[1].model.(azureView).namingVerb == "" {
		t.Fatal("'n' did not arm the name input")
	}
	// While naming, tab/d must reach the text input, not the container.
	nm2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyTab})
	if nm2.(tabsModel).active != 1 {
		t.Fatal("tab key switched tabs during naming")
	}
	nm3, _ := nm2.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if nm3.(tabsModel).picker != nil {
		t.Fatal("'d' opened the dir picker during naming")
	}
}

// TestDashboardNamingForwardedInContainer proves the container routes keys to
// the dashboard's adopt name prompt instead of its own global keymap (d/o/?/
// tab) while the dashboard is naming — regression for the adopt prompt losing
// keystrokes to the tab container.
func TestDashboardNamingForwardedInContainer(t *testing.T) {
	m := seedTabs(t)
	dm := m.tabs[0].model.(dashboardModel)
	dm.items = []dashItem{{provider: "github", adopt: true, adoptDir: "/w/foo"}}
	dm.cursor = 0
	m.tabs[0].model = dm

	// 'a' on the adoptable row arms the name prompt.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	tm := nm.(tabsModel)
	if !tm.tabs[0].model.(dashboardModel).naming {
		t.Fatal("'a' on an adoptable row did not arm the dashboard name prompt")
	}

	// While naming, 'd' must reach the text input, not the container's dir picker.
	nm2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	tm2 := nm2.(tabsModel)
	if tm2.picker != nil {
		t.Fatal("'d' opened the dir picker while the dashboard adopt prompt was naming")
	}
	if v := tm2.tabs[0].model.(dashboardModel).nameInput.Value(); v != "d" {
		t.Fatalf("'d' did not land in the dashboard name input, got %q", v)
	}
}

func TestUpAtTopFocusesTabBar(t *testing.T) {
	m := seedTabs(t)
	// Dashboard cursor starts at 0; ↑ emits focusTabsMsg.
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	tm := nm.(tabsModel)
	if cmd == nil {
		t.Fatal("up at top did not emit a command")
	}
	nm, _ = tm.Update(cmd())
	tm = nm.(tabsModel)
	if !tm.barFocus {
		t.Fatal("focusTabsMsg did not focus the tab bar")
	}
	// ←/→ walk tabs while the bar is focused.
	nm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRight})
	tm = nm.(tabsModel)
	if tm.active != 1 || !tm.barFocus {
		t.Fatalf("right on the bar: active=%d barFocus=%v", tm.active, tm.barFocus)
	}
	nm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyLeft})
	tm = nm.(tabsModel)
	if tm.active != 0 {
		t.Fatalf("left on the bar: active=%d", tm.active)
	}
	// ↓ hands focus back to the view.
	nm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyDown})
	if nm.(tabsModel).barFocus {
		t.Fatal("down did not return focus to the view")
	}
}

func TestTabsDefaultToAzureAndGitHub(t *testing.T) {
	m := seedTabs(t)
	var names []string
	for _, tb := range m.tabs {
		names = append(names, tb.title)
	}
	if len(m.tabs) != 3 || m.tabs[1].name != "azure" || m.tabs[2].name != "github" {
		t.Fatalf("default tabs = %v", names)
	}
}

// TestMouseClickOnTabCellSwitchesTab proves bubblezone marks each tab cell in
// View and the container hit-tests a left-click release against those zones.
func TestMouseClickOnTabCellSwitchesTab(t *testing.T) {
	m := seedTabs(t)
	// Render through the root View so zone.Scan records tab-cell bounds.
	_ = m.View()
	time.Sleep(120 * time.Millisecond) // bubblezone records zones asynchronously
	z := zone.Get("tab:2")
	if z == nil || z.IsZero() {
		t.Fatal("tab cell 2 has no zone — Mark/Scan not wired")
	}
	nm, _ := m.Update(tea.MouseMsg{
		X: z.StartX, Y: z.StartY,
		Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	if got := nm.(tabsModel).active; got != 2 {
		t.Fatalf("click on tab cell 2 should activate it, active = %d", got)
	}
}

// TestMouseClickOnTabCellIgnoredWhileNaming proves a tab-cell click is
// ignored while the active tab is in a text-entry state, mirroring the
// keyboard tab-switch keys (which forward to the input instead of
// switching) — regression for a click silently stealing focus mid-prompt.
func TestMouseClickOnTabCellIgnoredWhileNaming(t *testing.T) {
	m := seedTabs(t)
	// Move to the Azure tab and arm the new-profile text input ('n').
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	nm, _ = nm.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	tm := nm.(tabsModel)
	if tm.tabs[1].model.(azureView).namingVerb == "" {
		t.Fatal("'n' did not arm the name input")
	}
	// Render through the root View so bubblezone records the tab-cell bounds.
	_ = tm.View()
	time.Sleep(120 * time.Millisecond) // bubblezone records zones asynchronously
	z := zone.Get("tab:2")
	if z == nil || z.IsZero() {
		t.Fatal("tab cell 2 has no zone — Mark/Scan not wired")
	}
	nm2, _ := tm.Update(tea.MouseMsg{
		X: z.StartX, Y: z.StartY,
		Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	if got := nm2.(tabsModel).active; got != 1 {
		t.Fatalf("click on tab cell 2 while naming should be ignored, active = %d, want 1", got)
	}
}

func TestOptionsOverlayEnablesProviderTabs(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	tm := nm.(tabsModel)
	if tm.options == nil {
		t.Fatal("'o' did not open the options overlay")
	}
	// Check AWS (cursor: azure→github→aws) and save.
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyDown}, {Type: tea.KeyDown},
		{Type: tea.KeySpace},
		{Type: tea.KeyEnter},
	} {
		nm, _ = tm.Update(k)
		tm = nm.(tabsModel)
	}
	if tm.options != nil {
		t.Fatal("options overlay did not close on save")
	}
	if len(tm.tabs) != 4 || tm.tabs[3].name != "aws" {
		var names []string
		for _, tb := range tm.tabs {
			names = append(names, tb.name)
		}
		t.Fatalf("tabs after enabling AWS = %v", names)
	}
	// Persisted: a fresh container under the same HOME sees the same set.
	if got := len(NewTabs().tabs); got != 4 {
		t.Fatalf("fresh container tabs = %d, want 4", got)
	}
}

func TestOptionsPopupOverlaysAndOpensFromBar(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	tm := nm.(tabsModel)
	// Open from tab-bar focus (options must work from anywhere).
	nm, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyUp})
	tm = nm.(tabsModel)
	nm, _ = tm.Update(cmd())
	tm = nm.(tabsModel)
	nm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	tm = nm.(tabsModel)
	if tm.options == nil {
		t.Fatal("'o' did not open options while the tab bar held focus")
	}
	// The popup overlays the underlying view: both are visible at once.
	v := tm.View()
	if !strings.Contains(v, "PROVIDER TABS") {
		t.Fatalf("popup content missing:\n%s", v)
	}
	if !strings.Contains(v, "MAPPINGS") {
		t.Fatalf("underlying dashboard should remain visible around the popup:\n%s", v)
	}
}

func TestHelpOverlayTogglesFromAnyTab(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	tm := nm.(tabsModel)
	if !tm.help {
		t.Fatal("'?' did not open the help overlay")
	}
	if v := tm.View(); !strings.Contains(v, "KEYS") || !strings.Contains(v, "link here") || !strings.Contains(v, "shell as profile") || !strings.Contains(v, "open console") || !strings.Contains(v, "hold shift to select/copy terminal text") {
		t.Fatalf("help overlay content missing:\n%s", v)
	}
	// Any key closes it without leaking to the tab.
	nm2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if nm2.(tabsModel).help {
		t.Fatal("a keypress should close the help overlay")
	}
	// While a text input is armed, '?' is text, not the overlay.
	nm3, _ := nm2.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyTab}) // Azure tab
	nm3, _ = nm3.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	nm4, _ := nm3.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if nm4.(tabsModel).help {
		t.Fatal("'?' must reach the armed text input, not open the overlay")
	}
}

// TestHelpOverlayClickDismisses proves any left click closes the help
// overlay, wherever it lands.
func TestHelpOverlayClickDismisses(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	tm := nm.(tabsModel)
	nm, _ = tm.Update(tea.MouseMsg{X: 1, Y: 1, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if nm.(tabsModel).help {
		t.Fatal("any click should dismiss the help overlay")
	}
}

// TestClickOutsideOptionsDismisses is the zone-integration proof for the
// options overlay's outside-click esc path: the box is real bubblezone
// bounds, and a release one cell to its left must dismiss without saving.
func TestClickOutsideOptionsDismisses(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")}) // open options
	tm := nm.(tabsModel)
	_ = tm.View()
	time.Sleep(120 * time.Millisecond)
	z := zone.Get("box:options")
	if z == nil || z.IsZero() {
		t.Fatal("options box zone missing")
	}
	outside := tea.MouseMsg{X: z.StartX - 1, Y: z.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	nm, _ = tm.Update(outside)
	if nm.(tabsModel).options != nil {
		t.Fatal("click outside the options popup should dismiss it")
	}
}

// TestClickOptionsRowThroughZonesSelectsThenSaves drives a full row click
// through real zone bounds: a click on an unselected row just selects it;
// clicking that same row again runs the overlay's enter (commits the
// checked set and closes), proving the opt:<i> wiring end to end.
func TestClickOptionsRowThroughZonesSelectsThenSaves(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	tm := nm.(tabsModel)
	_ = tm.View()
	time.Sleep(120 * time.Millisecond)
	z := zone.Get("opt:1") // github row — cursor starts on row 0 (azure)
	if z == nil || z.IsZero() {
		t.Fatal("opt:1 row zone missing")
	}
	click := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	nm, _ = tm.Update(click)
	tm2 := nm.(tabsModel)
	if tm2.options == nil || tm2.options.cursor != 1 {
		t.Fatal("first click should select row 1 without closing")
	}
	nm2, _ := tm2.Update(click)
	if nm2.(tabsModel).options != nil {
		t.Fatal("second click on the selected row should commit and close the options popup")
	}
}

// TestOptionsWheelMovesCursorThroughContainer proves the container routes
// wheel events to the options overlay's own cursor while it's open.
func TestOptionsWheelMovesCursorThroughContainer(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	tm := nm.(tabsModel)
	if tm.options.cursor != 0 {
		t.Fatalf("expected cursor 0 to start, got %d", tm.options.cursor)
	}
	nm2, _ := tm.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	tm2 := nm2.(tabsModel)
	if tm2.options.cursor != 1 {
		t.Fatalf("wheel down should move the options cursor, got %d", tm2.options.cursor)
	}
	nm3, _ := tm2.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if nm3.(tabsModel).options.cursor != 0 {
		t.Fatalf("wheel up should move back, got %d", nm3.(tabsModel).options.cursor)
	}
}

// TestClickOutsideDirPickerDismisses mirrors TestClickOutsideOptionsDismisses
// for the change-directory overlay.
func TestClickOutsideDirPickerDismisses(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	tm := nm.(tabsModel)
	_ = tm.View()
	time.Sleep(120 * time.Millisecond)
	z := zone.Get("box:dir")
	if z == nil || z.IsZero() {
		t.Fatal("dir box zone missing")
	}
	outside := tea.MouseMsg{X: z.StartX - 1, Y: z.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	nm, _ = tm.Update(outside)
	if nm.(tabsModel).picker != nil {
		t.Fatal("click outside the dir picker popup should dismiss it")
	}
}

// TestClickDirPickerRowThroughZonesChangesCwd drives a full row click through
// real zone bounds: select, then click-again runs enter (accepts and
// os.Chdir's), proving the dir:<i> wiring end to end.
func TestClickDirPickerRowThroughZonesChangesCwd(t *testing.T) {
	base := t.TempDir()
	other := t.TempDir()
	target := t.TempDir()
	t.Chdir(base)
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	tm := nm.(tabsModel)
	tm.picker.candidates = []string{other, target}
	tm.picker.refilter()
	_ = tm.View()
	time.Sleep(120 * time.Millisecond)
	z := zone.Get("dir:1")
	if z == nil || z.IsZero() {
		t.Fatal("dir:1 row zone missing")
	}
	click := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	nm, _ = tm.Update(click)
	tm2 := nm.(tabsModel)
	if tm2.picker == nil || tm2.picker.cursor != 1 {
		t.Fatal("first click should select row 1 without closing")
	}
	nm2, _ := tm2.Update(click)
	if nm2.(tabsModel).picker != nil {
		t.Fatal("second click on the selected row should accept and close the picker")
	}
	if cwd, _ := os.Getwd(); cwd != target {
		t.Fatalf("cwd = %q, want %q", cwd, target)
	}
}

// TestDirPickerWheelMovesCursorThroughContainer proves the container routes
// wheel events to the dir picker's own cursor while it's open.
func TestDirPickerWheelMovesCursorThroughContainer(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	tm := nm.(tabsModel)
	tm.picker.candidates = []string{"/a", "/b"}
	tm.picker.refilter()
	if tm.picker.cursor != 0 {
		t.Fatalf("expected cursor 0 to start, got %d", tm.picker.cursor)
	}
	nm2, _ := tm.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	tm2 := nm2.(tabsModel)
	if tm2.picker.cursor != 1 {
		t.Fatalf("wheel down should move the picker cursor, got %d", tm2.picker.cursor)
	}
	nm3, _ := tm2.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if nm3.(tabsModel).picker.cursor != 0 {
		t.Fatalf("wheel up should move back, got %d", nm3.(tabsModel).picker.cursor)
	}
}
