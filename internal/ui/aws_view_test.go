package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
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

func TestUseHereHiddenWhenSelectedProfilePinsCwd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)
	pinned := t.TempDir()
	os.WriteFile(filepath.Join(pinned, ".awsprofile"), []byte("work\n"), 0o644)
	t.Chdir(pinned)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	av := nm.(awsView)
	out := av.View()
	if strings.Contains(out, "Use here") {
		t.Fatalf("Use here should be hidden for the cwd-pinned selection:\n%s", out)
	}
	if !strings.Contains(out, "ACTIONS (3)") {
		t.Fatalf("action count should drop to 3:\n%s", out)
	}
	// The 'u' accelerator is inert for this selection.
	nm, cmd := av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	if cmd != nil || nm.(awsView).status != "" {
		t.Fatalf("hidden accelerator must be inert (status=%q)", nm.(awsView).status)
	}
}

func TestNewProfilePromptsForNameThenExecsCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	av := nm.(awsView)
	if !av.naming || cmd != nil {
		t.Fatalf("'a' should open the name prompt (naming=%v)", av.naming)
	}
	for _, r := range "fresh" {
		nm, _ = av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		av = nm.(awsView)
	}
	nm, cmd = av.Update(tea.KeyMsg{Type: tea.KeyEnter})
	av = nm.(awsView)
	if av.naming || cmd == nil {
		t.Fatalf("enter should close the prompt and exec the create login (naming=%v cmd=%v)", av.naming, cmd)
	}
	// esc cancels a fresh prompt without exec.
	nm, _ = av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	nm, cmd = nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if nm.(awsView).naming || cmd != nil {
		t.Fatal("esc should cancel the prompt without exec")
	}
}

func TestSignInHiddenWhenSessionLive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	// Inject a live session for the selected profile (disk-only fixtures for
	// the SSO cache are heavyweight; the predicate is what matters here).
	v.statuses["work"] = provider.Status{ProfileName: "work", Identity: "123/Admin"}
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	out := nm.(awsView).View()
	if strings.Contains(out, "Sign in") {
		t.Fatalf("Sign in should hide for a live session:\n%s", out)
	}
	if !strings.Contains(out, "ACTIONS (3)") {
		t.Fatalf("action count should drop to 3:\n%s", out)
	}
}
