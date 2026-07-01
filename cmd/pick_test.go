package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/github"
)

// pickCmd returns a bare cobra command wired to a fed stdin and a captured
// stdout, for driving resolveLoginTarget deterministically.
func pickCmd(in string) (*cobra.Command, *bytes.Buffer) {
	c := &cobra.Command{}
	out := new(bytes.Buffer)
	c.SetOut(out)
	c.SetErr(out)
	c.SetIn(strings.NewReader(in))
	return c, out
}

// seedGhProfiles points HOME at a temp dir holding the named GitHub profiles
// (zero of them still creates the empty profiles dir).
func seedGhProfiles(t *testing.T, names ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	gp := filepath.Join(home, ".github-profiles")
	os.MkdirAll(gp, 0o755)
	for _, n := range names {
		os.WriteFile(filepath.Join(gp, n+".conf"), []byte("GH_HOST=github.com\nGH_USER="+n+"\n"), 0o644)
	}
	return home
}

// chdirClean moves into a fresh temp dir (no pointer file anywhere up the tree)
// so the picker never resolves a stray directory pin.
func chdirClean(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

// stubInteractive overrides the TTY check for the duration of the test.
func stubInteractive(t *testing.T, v bool) {
	t.Helper()
	orig := isInteractive
	isInteractive = func() bool { return v }
	t.Cleanup(func() { isInteractive = orig })
}

func TestConfirmCreateProfileAssumeYesSkipsPrompt(t *testing.T) {
	stubInteractive(t, false) // even non-interactive, --yes wins without prompting
	c, out := pickCmd("")
	if !confirmCreateProfile(c, "ghrl", "new", "github.com", true) {
		t.Fatal("assumeYes should return true")
	}
	if out.String() != "" {
		t.Fatalf("assumeYes must not prompt, got: %q", out.String())
	}
}

func TestConfirmCreateProfileNonInteractiveDeclines(t *testing.T) {
	stubInteractive(t, false)
	c, _ := pickCmd("")
	if confirmCreateProfile(c, "ghrl", "new", "github.com", false) {
		t.Fatal("non-interactive without --yes should return false")
	}
}

func TestConfirmCreateProfileInteractiveYes(t *testing.T) {
	stubInteractive(t, true)
	c, out := pickCmd("y\n")
	if !confirmCreateProfile(c, "ghrl", "new", "github.com", false) {
		t.Fatal(`interactive "y" should return true`)
	}
	if !strings.Contains(out.String(), `profile "new" doesn't exist`) {
		t.Fatalf("missing prompt: %q", out.String())
	}
}

func TestConfirmCreateProfileInteractiveDeclines(t *testing.T) {
	stubInteractive(t, true)
	for _, in := range []string{"n\n", "\n", ""} {
		c, _ := pickCmd(in)
		if confirmCreateProfile(c, "ghrl", "new", "github.com", false) {
			t.Fatalf("interactive %q should return false", in)
		}
	}
}

func TestResolveLoginTargetExplicitArg(t *testing.T) {
	seedGhProfiles(t, "work", "emu")
	c, _ := pickCmd("")
	got, err := resolveLoginTarget(c, github.NewProvider(), []string{"chosen"}, "ghrl")
	if err != nil || got != "chosen" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestResolveLoginTargetPin(t *testing.T) {
	seedGhProfiles(t, "work", "emu")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".ghprofile"), []byte("emu\n"), 0o644)
	old, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(old)
	c, _ := pickCmd("")
	got, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl")
	if err != nil || got != "emu" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestResolveLoginTargetNoProfiles(t *testing.T) {
	seedGhProfiles(t)
	chdirClean(t)
	c, _ := pickCmd("")
	_, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl")
	if err == nil || !strings.Contains(err.Error(), "no profiles yet") {
		t.Fatalf("want friendly no-profiles error, got %v", err)
	}
}

func TestResolveLoginTargetSingleAutoSelect(t *testing.T) {
	seedGhProfiles(t, "only")
	chdirClean(t)
	c, out := pickCmd("")
	got, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl")
	if err != nil || got != "only" {
		t.Fatalf("got %q err %v", got, err)
	}
	if !strings.Contains(out.String(), "using the only profile") {
		t.Fatalf("missing announce: %q", out.String())
	}
}

func TestResolveLoginTargetMultiNonInteractive(t *testing.T) {
	seedGhProfiles(t, "work", "emu")
	chdirClean(t)
	stubInteractive(t, false)
	c, _ := pickCmd("")
	_, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl")
	if err == nil || !strings.Contains(err.Error(), "multiple profiles") {
		t.Fatalf("want multiple-profiles error, got %v", err)
	}
	if !strings.Contains(err.Error(), "emu") || !strings.Contains(err.Error(), "work") {
		t.Fatalf("error should list names: %v", err)
	}
}

func TestResolveLoginTargetMultiInteractive(t *testing.T) {
	seedGhProfiles(t, "work", "emu")
	chdirClean(t)
	stubInteractive(t, true)
	// Profiles sort by name: [emu, work]; selecting 2 -> "work".
	c, out := pickCmd("2\n")
	got, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl")
	if err != nil || got != "work" {
		t.Fatalf("got %q err %v (out=%q)", got, err, out.String())
	}
	if !strings.Contains(out.String(), "Select a profile") {
		t.Fatalf("missing prompt: %q", out.String())
	}
}

func TestResolveLoginTargetReprompt(t *testing.T) {
	seedGhProfiles(t, "work", "emu")
	chdirClean(t)
	stubInteractive(t, true)
	// Invalid "abc" then valid "2" -> "work".
	c, _ := pickCmd("abc\n2\n")
	got, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl")
	if err != nil || got != "work" {
		t.Fatalf("got %q err %v", got, err)
	}
}

// TestResolveLoginTargetProviderAgnostic exercises the helper through the AWS
// provider to prove it is not GitHub-specific.
func TestResolveLoginTargetProviderAgnostic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "prod.conf"), []byte("AWS_SSO_START_URL=https://org.awsapps.com/start\n"), 0o644)
	chdirClean(t)
	c, out := pickCmd("")
	got, err := resolveLoginTarget(c, aws.NewProvider(), nil, "azrl aws")
	if err != nil || got != "prod" {
		t.Fatalf("got %q err %v", got, err)
	}
	if !strings.Contains(out.String(), "azrl aws: using the only profile") {
		t.Fatalf("missing announce: %q", out.String())
	}
}
