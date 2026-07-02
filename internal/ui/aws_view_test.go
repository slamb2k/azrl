package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	// Enter in the action pane runs the selected action (Sign in → status hint).
	nm2, _ := nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(nm2.(awsView).status, "azrl aws login") {
		t.Fatalf("enter in the action pane did not run the action: %q", nm2.(awsView).status)
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
