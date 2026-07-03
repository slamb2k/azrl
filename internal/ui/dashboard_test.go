package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/provider"
)

// clearAmbientEnv blanks every native-state env override so tests only see the
// fixture files under the temp HOME.
func clearAmbientEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"AZURE_CONFIG_DIR", "GH_CONFIG_DIR", "AWS_CONFIG_FILE", "AWS_PROFILE", "CLOUDSDK_CONFIG", "CLOUDSDK_ACTIVE_CONFIG_NAME"} {
		t.Setenv(k, "")
	}
}

// seedDashHome points HOME at a temp dir with one Azure and one GitHub profile,
// each carrying a LAST_USED so the sort order is deterministic.
func seedDashHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(filepath.Join(az, "acme"), 0o755)
	os.WriteFile(filepath.Join(az, "acme.conf"),
		[]byte("AZ_TENANT=acme.com\nLAST_USED=2026-06-30T10:00:00Z\nLAST_DIR=/work/acme\n"), 0o644)
	azProfile := filepath.Join(az, "acme", "azureProfile.json")
	os.WriteFile(azProfile,
		[]byte(`{"subscriptions":[{"user":{"name":"u@acme.com"},"isDefault":true,"tenantId":"g1"}]}`), 0o644)
	gh := filepath.Join(home, ".github-profiles")
	os.MkdirAll(filepath.Join(gh, "work"), 0o755)
	os.WriteFile(filepath.Join(gh, "work.conf"),
		[]byte("GH_HOST=github.com\nLAST_USED=2026-05-01T10:00:00Z\nLAST_DIR=/work/repo\n"), 0o644)
	ghHosts := filepath.Join(gh, "work", "hosts.yml")
	os.WriteFile(ghHosts, []byte("github.com:\n    user: octocat\n"), 0o644)

	// Status folds each token-cache file's mtime into LastUsed; pin the fixture
	// mtimes to their LAST_USED so the sort order stays deterministic (otherwise
	// the freshly-written files' "now" mtimes would decide ordering).
	azTime := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	ghTime := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	os.Chtimes(azProfile, azTime, azTime)
	os.Chtimes(ghHosts, ghTime, ghTime)
}

func sizedDashboard(t *testing.T) dashboardModel {
	t.Helper()
	m := newDashboard(provider.All())
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	return nm.(dashboardModel)
}

func TestDashboardRendersUnmappedSortedByLastUsed(t *testing.T) {
	seedDashHome(t)
	m := sizedDashboard(t)
	v := m.View()
	for _, want := range []string{"Dashboard", "MAPPINGS", "AMBIENT", "UNMAPPED PROFILES",
		"azure:acme", "github:work", "u@acme.com", "octocat@github.com"} {
		if !strings.Contains(v, want) {
			t.Fatalf("dashboard view missing %q:\n%s", want, v)
		}
	}
	// acme (2026-06-30) is more recent than work (2026-05-01) → sorts first.
	if strings.Index(v, "azure:acme") > strings.Index(v, "github:work") {
		t.Fatalf("unmapped rows not sorted by last-used desc:\n%s", v)
	}
}

func TestDashboardEmptyState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.MkdirAll(filepath.Join(home, ".github-profiles"), 0o755)
	m := sizedDashboard(t)
	v := m.View()
	// Every section renders its empty state; nothing panics at any width.
	for _, want := range []string{"MAPPINGS", "No mappings yet", "AMBIENT", "No native defaults detected",
		"UNMAPPED PROFILES", "No profiles yet"} {
		if !strings.Contains(v, want) {
			t.Fatalf("expected empty state %q:\n%s", want, v)
		}
	}
}

func TestDashboardMappingSectionAndScopeMarkers(t *testing.T) {
	seedDashHome(t)
	home := os.Getenv("HOME")
	work := filepath.Join(home, "work")
	sub := filepath.Join(work, "sub")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(work, ".azprofile"), []byte("acme\n"), 0o644)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "mappings"),
		[]byte(work+"\tacme\tpointer\n"), 0o644)

	// From the mapping's own dir the row carries ●.
	t.Chdir(work)
	v := sizedDashboard(t).View()
	if !strings.Contains(v, "azure:acme") || !strings.Contains(v, ".azprofile") {
		t.Fatalf("mapping row missing target/source icon:\n%s", v)
	}
	if !strings.Contains(v, "●") {
		t.Fatalf("cwd mapping should carry ●:\n%s", v)
	}

	// From a child dir the governing ancestor still carries the ● marker
	// (orange in a colour terminal; scope is colour-graded, not glyph-graded).
	t.Chdir(sub)
	v = sizedDashboard(t).View()
	if !strings.Contains(v, "●") {
		t.Fatalf("ancestor mapping should carry the ● marker:\n%s", v)
	}

	// Mapped profiles leave the unmapped section (AC-011).
	if strings.Contains(v, "azure:acme · ") {
		t.Fatalf("mapped profile still listed as unmapped:\n%s", v)
	}
}

func TestDashboardAmbientRows(t *testing.T) {
	seedDashHome(t)
	home := os.Getenv("HOME")
	ghNative := filepath.Join(home, ".config", "gh")
	os.MkdirAll(ghNative, 0o755)
	os.WriteFile(filepath.Join(ghNative, "hosts.yml"), []byte("github.com:\n    user: octocat\n"), 0o644)

	v := sizedDashboard(t).View()
	if !strings.Contains(v, "🌐") {
		t.Fatalf("ambient row missing 🌐 marker:\n%s", v)
	}
	// octocat@github.com matches the saved "work" profile's identity.
	if !strings.Contains(v, "managed") || strings.Contains(v, "unmanaged") {
		t.Fatalf("ambient row should carry the managed label:\n%s", v)
	}

	// An identity matching no profile renders an explicit unmanaged label.
	os.WriteFile(filepath.Join(ghNative, "hosts.yml"), []byte("github.com:\n    user: stranger\n"), 0o644)
	if v := sizedDashboard(t).View(); !strings.Contains(v, "unmanaged") {
		t.Fatalf("unmanaged ambient identity not labelled:\n%s", v)
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
	// Cursor starts on the first row: the most-recent unmapped Azure acme profile.
	if sw.provider != "azure" || sw.profile != "acme" {
		t.Fatalf("switchTabMsg = %+v", sw)
	}
}

func TestDashboardAdoptKeyDispatch(t *testing.T) {
	// An unmanaged mapping row hands [a] off to the provider's capture flow;
	// any other row ignores the key.
	m := dashboardModel{items: []dashItem{
		{provider: "github", adoptDir: "/home/u/oss/foo"},
		{provider: "azure", profile: "acme"},
	}}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if cmd == nil {
		t.Fatal("[a] on an unmanaged row produced no command")
	}
	m.cursor = 1
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}); cmd != nil {
		t.Fatal("[a] on a managed row should be a no-op")
	}
}

func TestAdoptArgsDefaultToDirName(t *testing.T) {
	cases := map[string][]string{
		"azure":  {"capture", "foo"},
		"github": {"gh", "capture", "foo"},
		"aws":    {"aws", "capture", "foo"},
		"gcp":    {"gcp", "capture", "foo"},
	}
	for prov, want := range cases {
		got := adoptArgs(prov, "/home/u/oss/Foo")
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("adoptArgs(%s) = %v, want %v", prov, got, want)
		}
	}
}

func TestDashboardTickReaggregates(t *testing.T) {
	seedDashHome(t)
	m := sizedDashboard(t)
	nm, cmd := m.Update(dashboardTickMsg{})
	if cmd == nil {
		t.Fatal("tick did not reschedule")
	}
	if len(nm.(dashboardModel).items) != 2 {
		t.Fatalf("tick did not re-aggregate: %d items", len(nm.(dashboardModel).items))
	}
}

func TestDashboardFSEventReaggregates(t *testing.T) {
	seedDashHome(t)
	m := sizedDashboard(t)
	if len(m.items) != 2 {
		t.Fatalf("setup: expected 2 items, got %d", len(m.items))
	}

	// Simulate an external change: a new Azure profile appears on disk. A real
	// fsnotify event would deliver an fsEventMsg; feed one synthetically so the
	// message-handling path is exercised deterministically (no reliance on timing).
	home := os.Getenv("HOME")
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(filepath.Join(az, "beta"), 0o755)
	os.WriteFile(filepath.Join(az, "beta.conf"),
		[]byte("AZ_TENANT=beta.com\nLAST_USED=2026-06-01T10:00:00Z\nLAST_DIR=/work/beta\n"), 0o644)

	nm, _ := m.Update(fsEventMsg{})
	if got := len(nm.(dashboardModel).items); got != 3 {
		t.Fatalf("fsEventMsg did not re-aggregate: got %d items, want 3", got)
	}
}

func TestDashboardWatcherFallsBackGracefully(t *testing.T) {
	// A dashboard with no watcher (create failure path) must still Init to the
	// timer and handle fsEventMsg without panicking.
	seedDashHome(t)
	m := newDashboard(provider.All())
	m.watcher = nil
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init returned nil cmd; timer fallback missing")
	}
	if _, cmd := m.Update(fsEventMsg{}); cmd != nil {
		t.Fatal("watcher-less fsEventMsg should not re-arm a watch")
	}
}

func TestDashboardNarrowWidthNoOverflow(t *testing.T) {
	seedDashHome(t)
	home := os.Getenv("HOME")
	work := filepath.Join(home, "work")
	os.MkdirAll(work, 0o755)
	os.WriteFile(filepath.Join(work, ".azprofile"), []byte("acme\n"), 0o644)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "mappings"),
		[]byte(work+"\tacme\tpointer\n"), 0o644)
	for _, w := range []int{20, 46, 80} {
		m := newDashboard(provider.All())
		nm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 34})
		for _, line := range strings.Split(nm.(dashboardModel).View(), "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Fatalf("width %d: line width %d exceeds terminal: %q", w, lw, line)
			}
		}
	}
}

func TestDashboardDriftAndExpiryRendering(t *testing.T) {
	future := time.Now().Add(42 * time.Minute)
	m := dashboardModel{width: 120, ov: Overview{
		Mappings: []MappingRow{{
			Provider: "azure", Title: "Azure", Dir: "/work/acme", Profile: "acme",
			Source: "pointer", Scope: ScopeNone, Drifted: true, Pointer: ".azprofile",
		}},
		Unmapped: []UnmappedRow{{Provider: "azure", Title: "Azure", Status: provider.Status{
			ProfileName: "fiig", Identity: "u@acme.com", Expiry: &future, LastUsed: time.Now(),
		}}},
	}}
	m.items = overviewItems(m.ov)
	v := m.View()
	if !strings.Contains(v, "⚠ drift") {
		t.Fatalf("drift marker missing:\n%s", v)
	}
	if !strings.Contains(v, "in ") {
		t.Fatalf("relative expiry missing:\n%s", v)
	}

	past := time.Now().Add(-time.Hour)
	m.ov.Unmapped[0].Status.Expiry = &past
	if !strings.Contains(m.View(), "expired") {
		t.Fatalf("expired expiry missing:\n%s", m.View())
	}
}

func TestDashboardHeaderShowsCwdAndHint(t *testing.T) {
	seedDashHome(t)
	v := sizedDashboard(t).View()
	if !strings.Contains(v, "📁") {
		t.Fatalf("dashboard header missing the current directory:\n%s", v)
	}
}

func TestDashboardHintPriorities(t *testing.T) {
	// Empty overview → onboarding nudge in the chip, no notice line.
	if short, notice := dashboardHints(Overview{}); !strings.Contains(short, "no directories pinned") || notice != "" {
		t.Fatalf("empty hints = %q / %q", short, notice)
	}
	// An unmanaged mapping outranks the all-good message.
	ov := Overview{Mappings: []MappingRow{{Dir: "/x", Unmanaged: "who@github.com"}}}
	if short, notice := dashboardHints(ov); !strings.Contains(short, "unmanaged") || !strings.Contains(notice, "who@github.com") {
		t.Fatalf("unmanaged hints = %q / %q", short, notice)
	}
	// Drift outranks unmanaged; the chip stays compact while the notice
	// carries both sides.
	ov.Mappings = append([]MappingRow{{Dir: "/y", Profile: "p", Drifted: true}}, ov.Mappings...)
	ov.Ambient = []AmbientRow{{Provider: "", Identity: ""}}
	short, notice := dashboardHints(ov)
	if !strings.Contains(short, "drift") || strings.Contains(short, "pinned to") {
		t.Fatalf("drift chip should stay compact: %q", short)
	}
	if !strings.Contains(notice, "the pin expects") {
		t.Fatalf("drift notice should explain the pin side: %q", notice)
	}
}

func TestDashboardHintNamesBothDriftSides(t *testing.T) {
	ov := Overview{
		Mappings: []MappingRow{{Provider: "azure", Dir: "/w/x", Profile: "fiig", Drifted: true}},
		Ambient:  []AmbientRow{{Provider: "azure", Identity: "u@velrada.com · velrada.com"}},
	}
	_, notice := dashboardHints(ov)
	for _, want := range []string{"drift", "u@velrada.com · velrada.com", "fiig"} {
		if !strings.Contains(notice, want) {
			t.Fatalf("drift notice missing %q: %q", want, notice)
		}
	}
}
