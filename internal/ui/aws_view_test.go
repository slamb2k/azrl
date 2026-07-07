package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/browserpick"
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

	for _, want := range []string{"AWS", "PROFILES", "work", "acme.awsapps.com", "Sign in", "Link here", "New profile", "Remove"} {
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

func TestProviderViewBrowserEscClearsStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	av := nm.(awsView)
	if av.status == "" {
		t.Fatal("expected a status message while discovery is pending")
	}

	// Manual-entry esc.
	nm2, _ := av.Update(browserProfilesMsg{forProfile: "work", err: os.ErrDeadlineExceeded})
	av2 := nm2.(awsView)
	nm3, _ := av2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	av3 := nm3.(awsView)
	if av3.status != "" {
		t.Fatalf("esc from manual entry left a stale status: %q", av3.status)
	}

	// Picker esc.
	nm, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	av = nm.(awsView)
	nm2, _ = av.Update(browserProfilesMsg{forProfile: "work", profiles: []browserpick.Profile{
		{Browser: "edge", OS: "linux", Dir: "Profile 2", Name: "Work", Email: "simon@acme.com"},
	}})
	av2 = nm2.(awsView)
	nm3, _ = av2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	av3 = nm3.(awsView)
	if av3.status != "" {
		t.Fatalf("esc from picker left a stale status: %q", av3.status)
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

func TestNewProfilePromptsForNameThenExecsCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	av := nm.(awsView)
	if !av.naming || cmd != nil {
		t.Fatalf("'n' should open the name prompt (naming=%v)", av.naming)
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
	nm, _ = av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	nm, cmd = nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if nm.(awsView).naming || cmd != nil {
		t.Fatal("esc should cancel the prompt without exec")
	}
}

func TestEmptyProviderShowsOnlyBootstrapAction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	os.MkdirAll(filepath.Join(home, ".aws-profiles"), 0o755)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	out := nm.(awsView).View()
	if !strings.Contains(out, "ACTIONS (1)") || !strings.Contains(out, "New profile") {
		t.Fatalf("empty provider should offer exactly New profile:\n%s", out)
	}
	for _, hidden := range []string{"Sign in", "Link here", "Remove"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("%q should hide with zero profiles:\n%s", hidden, out)
		}
	}
	// The bootstrap action works: 'n' opens the name prompt.
	nm, _ = nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if !nm.(awsView).naming {
		t.Fatal("'n' should open the new-profile prompt on an empty provider")
	}
}

func TestLinkHereDisabledWhenAlreadyLinked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)
	linked := t.TempDir()
	os.WriteFile(filepath.Join(linked, ".awsprofile"), []byte("work\n"), 0o644)
	t.Chdir(linked)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	av := nm.(awsView)
	out := av.View()
	// Never hidden: the verb stays listed, disabled, with its reason.
	if !strings.Contains(out, "Link here") || !strings.Contains(out, "already linked here") {
		t.Fatalf("Link here should render disabled with its reason:\n%s", out)
	}
	if !strings.Contains(out, "ACTIONS (5)") {
		t.Fatalf("action count must not drop when a verb is disabled:\n%s", out)
	}
	// The accelerator explains instead of running.
	nm, cmd := av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	if cmd != nil {
		t.Fatal("disabled accelerator must not run")
	}
	if !strings.Contains(nm.(awsView).status, "already linked here") {
		t.Fatalf("disabled accelerator should surface the reason, got %q", nm.(awsView).status)
	}
}

func TestSignInVisibleWithLiveSessionHint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	v.statuses["work"] = provider.Status{ProfileName: "work", Identity: "123/Admin"}
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	av := nm.(awsView)
	out := av.View()
	if !strings.Contains(out, "Sign in") || !strings.Contains(out, "re-auth anyway") {
		t.Fatalf("Sign in must stay visible for a live session, with the swapped hint:\n%s", out)
	}
	// Still runnable — re-auth is idempotent.
	_, cmd := av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("Sign in on a live session must still return the handoff command")
	}
}

func TestRefreshKeysReload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)

	v := newAwsView()
	os.WriteFile(filepath.Join(ap, "late.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)
	nm, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if len(nm.(awsView).profiles) != 1 {
		t.Fatal("'r' did not reload the profile list")
	}
}
