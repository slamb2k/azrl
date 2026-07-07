package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFuzzyScore(t *testing.T) {
	if fuzzyScore("xyz", "~/work/azrl") >= 0 {
		t.Fatal("non-subsequence must not match")
	}
	if fuzzyScore("", "anything") != 0 {
		t.Fatal("empty pattern matches everything at score 0")
	}
	// Segment-start matches outrank buried ones.
	if fuzzyScore("az", "~/work/azrl") <= fuzzyScore("az", "~/hazmat/x") {
		t.Fatal("segment-start match should outscore a buried match")
	}
}

func TestDirDisplayExpandRoundTrip(t *testing.T) {
	home, _ := os.UserHomeDir()
	d := filepath.Join(home, "work", "x")
	if got := displayDir(d); got != "~/work/x" {
		t.Fatalf("displayDir = %q", got)
	}
	if got := expandDir("~/work/x"); got != d {
		t.Fatalf("expandDir = %q, want %q", got, d)
	}
}

func TestDirPickerAcceptsLiteralPath(t *testing.T) {
	target := t.TempDir()
	p := newDirPicker(80, 24)
	p.candidates = nil // force the literal-path fallback
	p.refilter()
	for _, r := range target {
		p, _, _ = p.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// No candidates match a fresh temp dir, so enter falls back to the literal.
	_, picked, closed := p.update(tea.KeyMsg{Type: tea.KeyEnter})
	if !closed || picked != filepath.Clean(target) {
		t.Fatalf("picked=%q closed=%v, want %q", picked, closed, target)
	}
}

func TestTabsDirPickerChangesCwdEverywhere(t *testing.T) {
	seedTabs(t)
	base := t.TempDir()
	target := t.TempDir()
	t.Chdir(base)

	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	tm := nm.(tabsModel)
	if tm.picker == nil {
		t.Fatal("'d' did not open the dir picker")
	}
	for _, r := range target {
		nm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		tm = nm.(tabsModel)
	}
	nm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = nm.(tabsModel)
	if tm.picker != nil {
		t.Fatal("picker did not close on accept")
	}
	if cwd, _ := os.Getwd(); cwd != target {
		t.Fatalf("cwd = %q, want %q", cwd, target)
	}
	// The provider headers show the new directory (no bottom-bar echo).
	nm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyTab}) // move off the dashboard
	nm, _ = nm.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyTab})
	tm = nm.(tabsModel)
	if v := tm.tabs[2].model.(githubView).View(); !strings.Contains(v, displayDir(target)) {
		t.Fatalf("github header does not show the new dir %q", displayDir(target))
	}
}

func TestProviderViewShowsDetailPaneAndLegend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	out := nm.(awsView).View()

	for _, want := range []string{"DETAILS", "Identity", "Last used", "⌁ default", "map to a local browser profile", "🟠"} {
		if !strings.Contains(out, want) {
			t.Fatalf("provider view missing %q:\n%s", want, out)
		}
	}
}
