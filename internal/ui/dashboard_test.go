package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// clearAmbientEnv blanks every native-state env override so tests only see the
// fixture files under the temp HOME.
func clearAmbientEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"AZURE_CONFIG_DIR", "GH_CONFIG_DIR", "AWS_CONFIG_FILE", "AWS_PROFILE", "CLOUDSDK_CONFIG", "CLOUDSDK_ACTIVE_CONFIG_NAME", "AZRL_PROFILE"} {
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

func TestDashboardAdoptOpensNamePrompt(t *testing.T) {
	// [a] on an adoptable row opens the name prompt prefilled from the row's
	// dir; enter hands off to capture; any other row ignores the key.
	m := dashboardModel{width: 100, items: []dashItem{
		{provider: "github", adopt: true, adoptDir: "/home/u/oss/foo"},
		{provider: "azure", profile: "acme"},
	}}
	mod, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = mod.(dashboardModel)
	if cmd != nil {
		t.Fatal("[a] should open the prompt, not exec immediately")
	}
	if !m.naming || m.nameInput.Placeholder != "foo" {
		t.Fatalf("naming=%v placeholder=%q, want prompt prefilled with dir basename", m.naming, m.nameInput.Placeholder)
	}
	if v := m.View(); !strings.Contains(v, "Name for the adopted profile:") {
		t.Fatalf("prompt missing from view:\n%s", v)
	}
	// Enter with the empty input falls back to the placeholder and execs.
	mod, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mod.(dashboardModel)
	if cmd == nil || m.naming {
		t.Fatalf("enter should exec the capture handoff and close the prompt (cmd=%v naming=%v)", cmd, m.naming)
	}
}

func TestDashboardAdoptPromptEscCancels(t *testing.T) {
	m := dashboardModel{width: 100, items: []dashItem{{provider: "github", adopt: true, adoptDir: "/w/foo"}}}
	mod, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = mod.(dashboardModel)
	mod, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mod.(dashboardModel)
	if m.naming || cmd != nil {
		t.Fatal("esc should close the prompt without running anything")
	}
}

func TestDashboardAdoptIgnoredOnManagedRow(t *testing.T) {
	m := dashboardModel{width: 100, items: []dashItem{{provider: "azure", profile: "acme"}}}
	mod, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = mod.(dashboardModel)
	if cmd != nil || m.naming {
		t.Fatal("[a] on a managed row should be a no-op")
	}
}

func TestCaptureArgsPerProvider(t *testing.T) {
	cases := []struct {
		provider string
		want     []string
	}{
		{"azure", []string{"capture", "foo"}},
		{"github", []string{"gh", "capture", "foo"}},
		{"aws", []string{"aws", "capture", "foo"}},
		{"gcp", []string{"gcp", "capture", "foo"}},
	}
	for _, c := range cases {
		got := captureArgs(c.provider, "foo")
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Fatalf("captureArgs(%s) = %v, want %v", c.provider, got, c.want)
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
	future := time.Now().Add(10 * time.Minute)
	m := dashboardModel{width: 120, ov: Overview{
		Mappings: []MappingRow{
			{
				Provider: "azure", Title: "Azure", Dir: "/work/acme", Profile: "acme",
				Source: "pointer", Scope: ScopeNone, Drifted: true, Pointer: ".azprofile",
			},
			// AWS is the only provider whose expiry is actionable guidance.
			{
				Provider: "aws", Title: "AWS", Dir: "/work/api", Profile: "prod",
				Source: "pointer", Scope: ScopeNone, Pointer: ".awsprofile", Expiry: &future,
			},
		},
	}}
	m.items = overviewItems(m.ov)
	v := m.View()
	if !strings.Contains(v, "⚠ drift") {
		t.Fatalf("drift marker missing:\n%s", v)
	}
	if !strings.Contains(v, "⚠ expires in") {
		t.Fatalf("relative expiry missing:\n%s", v)
	}

	past := time.Now().Add(-time.Hour)
	m.ov.Mappings[1].Expiry = &past
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
	if short, notice := dashboardHints(Overview{}); !strings.Contains(short, "no directories linked") || notice != "" {
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
	if !strings.Contains(short, "drift") || strings.Contains(short, "linked to") {
		t.Fatalf("drift chip should stay compact: %q", short)
	}
	if !strings.Contains(notice, "the link expects") {
		t.Fatalf("drift notice should explain the link side: %q", notice)
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

func TestDashboardMappingRowShowsExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	m := dashboardModel{width: 120, ov: Overview{
		Mappings: []MappingRow{
			// Azure's access token refreshes silently on next use — never a row marker.
			{Provider: "azure", Title: "Azure", Dir: "/work/acme", Profile: "acme",
				Source: "pointer", Scope: ScopeNone, Pointer: ".azprofile", Expiry: &past},
			// The AWS SSO session dying is real guidance.
			{Provider: "aws", Title: "AWS", Dir: "/work/api", Profile: "prod",
				Source: "pointer", Scope: ScopeNone, Pointer: ".awsprofile", Expiry: &past},
		},
	}}
	m.items = overviewItems(m.ov)
	v := m.View()
	if strings.Count(v, "⚠ expired") != 1 {
		t.Fatalf("expected exactly one expired marker (aws only):\n%s", v)
	}
}

func TestDashboardHintExpiredGoverningPin(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	ov := Overview{Mappings: []MappingRow{
		{Provider: "aws", Dir: "/w/x", Profile: "acme", Scope: ScopeCwd, Expiry: &past},
		{Dir: "/w/y", Unmanaged: "who@github.com"},
	}}
	// An expired governing pin outranks an unmanaged identity.
	short, notice := dashboardHints(ov)
	if !strings.Contains(short, "expired") || !strings.Contains(short, "aws:acme") {
		t.Fatalf("expired pin chip = %q", short)
	}
	if !strings.Contains(notice, "sign in") {
		t.Fatalf("expired pin notice should point at sign in: %q", notice)
	}
	// Drift still outranks an expired pin.
	ov.Mappings = append([]MappingRow{{Dir: "/w/z", Profile: "p", Drifted: true}}, ov.Mappings...)
	if short, _ := dashboardHints(ov); !strings.Contains(short, "drift") {
		t.Fatalf("drift should outrank the expired pin: %q", short)
	}
}

func TestDashboardHintIgnoresExpiredNonGoverningMapping(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	ov := Overview{Mappings: []MappingRow{
		{Provider: "azure", Dir: "/w/x", Profile: "acme", Scope: ScopeNone, Expiry: &past},
	}}
	if short, _ := dashboardHints(ov); strings.Contains(short, "expired") {
		t.Fatalf("a pin that does not govern the cwd should not raise the expired hint: %q", short)
	}
}

func TestOverviewItemsMarksUnmanagedAmbientAdoptable(t *testing.T) {
	ov := Overview{Ambient: []AmbientRow{
		{Provider: "aws", Identity: "111122223333/Dev"},            // unmanaged
		{Provider: "azure", Identity: "me@x.com", Profile: "acme"}, // managed
	}}
	items := overviewItems(ov)
	if !items[0].adopt || items[0].adoptDir != "" {
		t.Fatalf("unmanaged ambient row should be adoptable with cwd prefill: %+v", items[0])
	}
	if items[1].adopt {
		t.Fatalf("managed ambient row must not be adoptable: %+v", items[1])
	}
}

func TestAmbientLineOffersAdoptOnUnmanaged(t *testing.T) {
	line := ambientLine(AmbientRow{Provider: "aws", Title: "AWS", Identity: "1111/Dev", Source: "file:~/.aws/config"}, 10, 20, 20)
	if !strings.Contains(line, "[a]dopt") {
		t.Fatalf("unmanaged ambient line missing [a]dopt: %q", line)
	}
	managed := ambientLine(AmbientRow{Provider: "aws", Title: "AWS", Identity: "1111/Dev", Source: "s", Profile: "prod"}, 10, 20, 20)
	if strings.Contains(managed, "[a]dopt") {
		t.Fatalf("managed ambient line must not offer adopt: %q", managed)
	}
}

func TestDashboardAdoptAmbientPrefillsCwdBasename(t *testing.T) {
	m := dashboardModel{width: 100, items: []dashItem{{provider: "aws", adopt: true}}}
	mod, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = mod.(dashboardModel)
	cwd, _ := os.Getwd()
	if !m.naming || m.nameInput.Placeholder != profile.DefaultName("", cwd) {
		t.Fatalf("ambient adopt should prefill from cwd: naming=%v placeholder=%q", m.naming, m.nameInput.Placeholder)
	}
}

func TestMappingRowExpiringSoonAmberAwsOnly(t *testing.T) {
	soon := time.Now().Add(10 * time.Minute)
	awsLine := mappingLine(MappingRow{Provider: "aws", Dir: "/w/a", Profile: "prod",
		Source: "pointer", Scope: ScopeCwd, Expiry: &soon}, 20, 20)
	if !strings.Contains(awsLine, "⚠ expires in") {
		t.Fatalf("aws row expiring soon missing amber marker: %q", awsLine)
	}
	azLine := mappingLine(MappingRow{Provider: "azure", Dir: "/w/a", Profile: "acme",
		Source: "pointer", Scope: ScopeCwd, Expiry: &soon}, 20, 20)
	if strings.Contains(azLine, "expires") || strings.Contains(azLine, "expired") {
		t.Fatalf("azure row must never show expiry: %q", azLine)
	}
	farAws := time.Now().Add(3 * time.Hour)
	quiet := mappingLine(MappingRow{Provider: "aws", Dir: "/w/a", Profile: "prod",
		Source: "pointer", Scope: ScopeCwd, Expiry: &farAws}, 20, 20)
	if strings.Contains(quiet, "expires") {
		t.Fatalf("healthy aws row must show nothing: %q", quiet)
	}
}

func TestUnmappedRowShowsNoExpiry(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	line := unmappedLine(UnmappedRow{Provider: "aws",
		Status: provider.Status{ProfileName: "stale", Identity: "1111/Dev", Expiry: &past}})
	if strings.Contains(line, "expired") || strings.Contains(line, "in ") {
		t.Fatalf("unmapped rows are not in play — no expiry display: %q", line)
	}
}

func TestExpiredHintGatedToAws(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	ov := Overview{Mappings: []MappingRow{
		{Provider: "azure", Dir: "/w/x", Profile: "acme", Scope: ScopeCwd, Expiry: &past},
	}}
	if short, _ := dashboardHints(ov); strings.Contains(short, "expired") {
		t.Fatalf("azure expiry must not drive the hint: %q", short)
	}
	ov.Unmapped = []UnmappedRow{{Provider: "gcp",
		Status: provider.Status{ProfileName: "old", Expiry: &past}}}
	if short, _ := dashboardHints(ov); strings.Contains(short, "expired") {
		t.Fatalf("gcp unmapped expiry must not drive the hint: %q", short)
	}
}

func TestDashboardRowVerbsBuildHandoffs(t *testing.T) {
	m := dashboardModel{width: 100, items: []dashItem{{provider: "aws", profile: "prod"}}}
	for _, key := range []string{"s", "t", "c"} {
		mod, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = mod.(dashboardModel)
		if cmd == nil {
			t.Fatalf("%q on a managed row should hand off", key)
		}
	}
}

func TestDashboardRowVerbsExplainOnUnmanagedRow(t *testing.T) {
	// An ambient row with no managed profile can't sign in/shell/console/link.
	m := dashboardModel{width: 100, items: []dashItem{{provider: "aws", adopt: true}}}
	for _, key := range []string{"s", "t", "c", "u"} {
		mod, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = mod.(dashboardModel)
		if cmd != nil {
			t.Fatalf("%q on an unmanaged row must not exec", key)
		}
	}
	if m.status == "" || !strings.Contains(m.status, "adopt") {
		t.Fatalf("unmanaged-row verb should explain itself in the status line: %q", m.status)
	}
}

func TestDashboardLinkHereUsesProviderInProcess(t *testing.T) {
	seedDashHome(t)
	work := filepath.Join(os.Getenv("HOME"), "work")
	os.MkdirAll(work, 0o755)
	t.Chdir(work)
	m := newDashboard(provider.All())
	// Find the azure acme item seedDashHome creates.
	idx := -1
	for i, it := range m.items {
		if it.provider == "azure" && it.profile == "acme" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("no azure:acme item: %+v", m.items)
	}
	m.cursor = idx
	mod, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = mod.(dashboardModel)
	if b, err := os.ReadFile(filepath.Join(work, ".azprofile")); err != nil || strings.TrimSpace(string(b)) != "acme" {
		t.Fatalf("u should link the cwd to acme: %q, %v", b, err)
	}
	if !strings.Contains(m.status, "linked") {
		t.Fatalf("status should confirm the link: %q", m.status)
	}
}

func TestDashboardCursorStartsOnGoverningRow(t *testing.T) {
	ov := Overview{Mappings: []MappingRow{
		{Provider: "github", Dir: "/w/other", Profile: "oss", Scope: ScopeNone},
		{Provider: "azure", Dir: "/w/here", Profile: "acme", Scope: ScopeCwd},
	}}
	if got := governingIndex(ov); got != 1 {
		t.Fatalf("governingIndex = %d, want 1", got)
	}
	if got := governingIndex(Overview{}); got != 0 {
		t.Fatalf("empty overview should default to 0, got %d", got)
	}
}

func TestDashboardSurfacesOpDone(t *testing.T) {
	m := dashboardModel{width: 100}
	mod, _ := m.Update(opDoneMsg{msg: "shell exited"})
	if got := mod.(dashboardModel).status; !strings.Contains(got, "shell exited") {
		t.Fatalf("opDoneMsg should surface in the dashboard status: %q", got)
	}
}

func TestAmbientLineExpiryTagsAwsOnly(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	soon := time.Now().Add(5 * time.Minute)
	expiredAws := ambientLine(AmbientRow{Provider: "aws", Title: "AWS", Identity: "1/Dev",
		Source: "s", Profile: "prod", Expiry: &past}, 10, 20, 20)
	if !strings.Contains(expiredAws, "⚠ expired") {
		t.Fatalf("expired aws default missing tag: %q", expiredAws)
	}
	soonAws := ambientLine(AmbientRow{Provider: "aws", Title: "AWS", Identity: "1/Dev",
		Source: "s", Profile: "prod", Expiry: &soon}, 10, 20, 20)
	if !strings.Contains(soonAws, "⚠ expires in") {
		t.Fatalf("soon-expiring aws default missing amber tag: %q", soonAws)
	}
	azure := ambientLine(AmbientRow{Provider: "azure", Title: "Azure", Identity: "me@x",
		Source: "s", Profile: "acme", Expiry: &past}, 10, 20, 20)
	if strings.Contains(azure, "expire") {
		t.Fatalf("azure default must never show expiry: %q", azure)
	}
}
