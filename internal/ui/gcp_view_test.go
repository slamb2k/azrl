package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestGcpViewRendersProfilesAndActions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	gp := filepath.Join(home, ".gcp-profiles")
	os.MkdirAll(gp, 0o755)
	os.WriteFile(filepath.Join(gp, "work.conf"),
		[]byte("GCP_CONFIG_NAME=work\nGCP_PROJECT=acme-prod\n"), 0o644)

	v := newGcpView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := nm.(gcpView).View()

	for _, want := range []string{"Google Cloud", "PROFILES", "work", "acme-prod", "Sign in", "Use here", "New profile", "Remove"} {
		if !strings.Contains(out, want) {
			t.Fatalf("GCP view missing %q:\n%s", want, out)
		}
	}
	// GCP has no active-profile file, so it must not offer a Switch action.
	if strings.Contains(out, "Switch") {
		t.Fatalf("GCP view should not offer a Switch action:\n%s", out)
	}
}

func TestGcpViewSurvivesDashboardMessages(t *testing.T) {
	if a, _ := newGcpView().Update(dashboardTickMsg{}); func() bool { _, ok := a.(gcpView); return !ok }() {
		t.Fatal("gcpView did not survive dashboardTickMsg")
	}
	if a, _ := newGcpView().Update(switchTabMsg{provider: "gcp"}); func() bool { _, ok := a.(gcpView); return !ok }() {
		t.Fatal("gcpView did not survive switchTabMsg")
	}
}
