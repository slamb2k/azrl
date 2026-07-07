package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/browserpick"
)

// seedAzure returns a sized azure view with one profile on disk.
func seedAzure(t *testing.T) azureView {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	v := newAzureView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	return nm.(azureView)
}

func TestAzureViewRendersUnifiedActions(t *testing.T) {
	v := seedAzure(t)
	out := v.View()
	for _, want := range []string{"Azure", "PROFILES (1)", "acme", "Sign in", "Link here", "New profile", "Browser profile", "Remove"} {
		if !strings.Contains(out, want) {
			t.Fatalf("azure view missing %q:\n%s", want, out)
		}
	}
	// Retired TUI verbs and the demoted everyday Capture must be gone.
	for _, gone := range []string{"Edit", "Rename", "Capture session"} {
		if strings.Contains(out, gone) {
			t.Fatalf("azure view still offers %q:\n%s", gone, out)
		}
	}
}

func TestAzureSignInHotkeyReturnsHandoff(t *testing.T) {
	v := seedAzure(t)
	_, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("'s' should return the login handoff command")
	}
}

func TestAzureNewProfilePromptExecsCreateLogin(t *testing.T) {
	v := seedAzure(t)
	nm, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	av := nm.(azureView)
	if av.namingVerb != "login" {
		t.Fatalf("'n' should open the new-profile prompt, verb=%q", av.namingVerb)
	}
	for _, r := range "fresh" {
		nm, _ = av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		av = nm.(azureView)
	}
	nm, cmd := av.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if nm.(azureView).namingVerb != "" || cmd == nil {
		t.Fatal("enter should close the prompt and exec the create login")
	}
}

func TestAzureEmptyStateOffersOnboardingPair(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	v := newAzureView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	out := nm.(azureView).View()
	if !strings.Contains(out, "ACTIONS (2)") || !strings.Contains(out, "Capture session") {
		t.Fatalf("empty azure tab should offer the onboarding pair:\n%s", out)
	}
}

func TestAzureDriftNoticeMentionsEnvrc(t *testing.T) {
	v := seedAzure(t)
	nm, _ := v.Update(identityMsg{
		who:        "u@fiig.com.au · fiig.com.au",
		ambientWho: "u@fiig.com.au · velrada.com",
		drift:      true,
	})
	av := nm.(azureView)
	got := av.identityStrip()
	if !strings.Contains(got, ".envrc") {
		t.Fatalf("drift strip should offer .envrc: %q", got)
	}
	for _, want := range []string{"velrada.com", "expects u@fiig.com.au", "fiig.com.au"} {
		if !strings.Contains(got, want) {
			t.Fatalf("drift strip missing %q: %q", want, got)
		}
	}
	// Clearing drift clears the notice.
	nm, _ = av.Update(identityMsg{who: "u@fiig.com.au · fiig.com.au"})
	if strings.Contains(nm.(azureView).identityStrip(), ".envrc") {
		t.Fatal("no drift should not mention .envrc")
	}
}

func TestAzureEnvrcHotkeyNeedsProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	t.Chdir(t.TempDir()) // no .azprofile anywhere up the tree
	v := newAzureView()
	nm, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if cmd != nil {
		t.Fatal("e without a resolved profile should not run a command")
	}
	if !strings.Contains(nm.(azureView).status, "no profile") {
		t.Fatalf("expected a 'no profile' status, got %q", nm.(azureView).status)
	}
}

func TestAzureBrowserActionOpensPickerAndWritesKeys(t *testing.T) {
	v := seedAzure(t)
	confPath := filepath.Join(os.Getenv("HOME"), ".azure-profiles", "acme.conf")
	nm, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	av := nm.(azureView)
	if av.browserFor != "acme" || cmd == nil {
		t.Fatalf("'b' did not arm discovery (for=%q)", av.browserFor)
	}
	nm, _ = av.Update(browserProfilesMsg{forProfile: "acme", profiles: []browserpick.Profile{
		{Browser: "edge", OS: "linux", Dir: "Profile 2", Name: "Work", Email: "simon@acme.com"},
	}})
	av = nm.(azureView)
	if av.browserPick == nil {
		t.Fatal("browser profiles msg did not open the picker")
	}
	nm, _ = av.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if nm.(azureView).browserPick != nil {
		t.Fatal("picker should close after enter")
	}
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), `AZ_BROWSER_CMD=microsoft-edge --profile-directory="Profile 2"`) ||
		!strings.Contains(string(b), "AZ_BROWSER_LABEL=Edge — Work") {
		t.Fatalf("keys not written:\n%s", b)
	}
}

func TestAzureRefreshKeysRecheckIdentity(t *testing.T) {
	v := seedAzure(t)
	nm, _ := v.Update(identityMsg{
		who:        "u@fiig.com.au · fiig.com.au",
		ambientWho: "u@fiig.com.au · velrada.com",
		drift:      true,
	})
	av := nm.(azureView)
	if !strings.Contains(av.identityStrip(), ".envrc") {
		t.Fatal("setup: expected drift notice before refresh")
	}
	// 'r' and 'f5' must re-check the identity, not just reload the profile
	// list, or an explicit refresh leaves a stale drift notice on screen.
	_, cmd := av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("'r' should batch identityCmd()")
	}
	_, cmd = av.Update(tea.KeyMsg{Type: tea.KeyF5})
	if cmd == nil {
		t.Fatal("'f5' should batch identityCmd()")
	}
}

func TestGroupArgsTopLevel(t *testing.T) {
	if got := strings.Join(groupArgs("", "login", "acme"), " "); got != "login acme" {
		t.Fatalf("groupArgs(\"\") = %q", got)
	}
	if got := cliGroup("azure"); got != "" {
		t.Fatalf("cliGroup(azure) = %q, want \"\"", got)
	}
}
