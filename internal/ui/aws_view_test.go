package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/profile"
)

func TestAwsViewRendersProfilesAndActions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := nm.(awsView).View()

	for _, want := range []string{"AWS", "PROFILES", "work", "acme.awsapps.com", "Sign in", "Use here", "New profile", "Remove"} {
		if !strings.Contains(out, want) {
			t.Fatalf("AWS view missing %q:\n%s", want, out)
		}
	}
	// AWS has no active-profile file, so it must not offer a Switch action.
	if strings.Contains(out, "Switch") {
		t.Fatalf("AWS view should not offer a Switch action:\n%s", out)
	}
}

func TestAwsViewSurvivesDashboardMessages(t *testing.T) {
	if a, _ := newAwsView().Update(dashboardTickMsg{}); func() bool { _, ok := a.(awsView); return !ok }() {
		t.Fatal("awsView did not survive dashboardTickMsg")
	}
	if a, _ := newAwsView().Update(switchTabMsg{provider: "aws"}); func() bool { _, ok := a.(awsView); return !ok }() {
		t.Fatal("awsView did not survive switchTabMsg")
	}
}

func TestProviderViewEnterOpensActionsEscReturns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if nm.(awsView).focus != focusActions {
		t.Fatal("enter on the profile pane did not open the action pane")
	}
	// Enter in the action pane runs the selected action (Sign in → an exec
	// handoff command for the interactive login flow).
	nm2, cmd := nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter in the action pane did not return the sign-in handoff command")
	}
	nm3, _ := nm2.(awsView).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if nm3.(awsView).focus != focusProfiles {
		t.Fatal("esc did not return focus to the profile pane")
	}
}

func TestProviderViewDeleteKeyRemoves(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.KeyMsg{Type: tea.KeyDelete})
	av := nm.(awsView)
	if !strings.Contains(av.status, "removed") {
		t.Fatalf("delete key did not remove the profile: %q", av.status)
	}
	if len(av.profiles) != 0 {
		t.Fatalf("profile list not reloaded after remove: %+v", av.profiles)
	}
}

func TestRenderProfilePaneScopeGlyphs(t *testing.T) {
	profiles := []profile.Listed{
		{Name: "work", Detail: "acme.awsapps.com"},
		{Name: "staging", Detail: "acme.awsapps.com"},
		{Name: "personal", Detail: "personal.awsapps.com"},
		{Name: "idle", Detail: "idle.awsapps.com"},
	}
	scopes := map[string]string{"work": ScopeCwd, "staging": ScopeAncestor, "personal": scopeGlobal, "idle": scopeElsewhere}
	out := renderProfilePane(profiles, 0, selActive, true, 40, scopes)
	if !strings.Contains(out, "●  work") || !strings.Contains(out, "●  staging") {
		t.Fatalf("dir-pinned profiles missing leading ● icon:\n%s", out)
	}
	// Only the global default carries 🌐; not-applicable rows get a grey ●.
	if !strings.Contains(out, "🌐 personal") {
		t.Fatalf("global-default profile missing 🌐 icon:\n%s", out)
	}
	if !strings.Contains(out, "●  idle") || strings.Contains(out, "🌐 idle") {
		t.Fatalf("mapped-elsewhere profile should carry the ● icon:\n%s", out)
	}
}

func TestGroupArgs(t *testing.T) {
	if got := strings.Join(groupArgs("aws", "login", "work"), " "); got != "aws login work" {
		t.Fatalf("groupArgs(aws) = %q", got)
	}
	// The test binary is not ghrl, so the gh group keeps its prefix.
	if got := strings.Join(groupArgs("gh", "login"), " "); got != "gh login" {
		t.Fatalf("groupArgs(gh) = %q", got)
	}
}
