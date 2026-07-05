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
	"github.com/slamb2k/azrl/internal/profile"
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
	seedGlobalConf(t, home)
	return home
}

// seedGlobalConf writes a minimal valid local-mode azrl.conf under HOME so login
// commands that route through loadGlobalOrSetup don't trip the setup nudge.
func seedGlobalConf(t *testing.T, home string) {
	t.Helper()
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "azrl.conf"), []byte("BROWSER_CMD=wslview\n"), 0o644)
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
	got, _, err := resolveLoginTarget(c, github.NewProvider(), []string{"chosen"}, "ghrl", validGhName)
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
	got, _, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl", validGhName)
	if err != nil || got != "emu" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestResolveLoginTargetNoProfiles(t *testing.T) {
	seedGhProfiles(t)
	chdirClean(t)
	c, _ := pickCmd("")
	_, _, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl", validGhName)
	if err == nil || !strings.Contains(err.Error(), "no profiles yet") {
		t.Fatalf("want friendly no-profiles error, got %v", err)
	}
}

func TestResolveLoginTargetSingleAutoSelect(t *testing.T) {
	seedGhProfiles(t, "only")
	chdirClean(t)
	c, out := pickCmd("")
	got, _, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl", validGhName)
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
	_, _, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl", validGhName)
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
	got, _, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl", validGhName)
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
	got, _, err := resolveLoginTarget(c, github.NewProvider(), nil, "ghrl", validGhName)
	if err != nil || got != "work" {
		t.Fatalf("got %q err %v", got, err)
	}
}

// TestResolveLoginTargetFirstLoginDefault proves that with zero saved profiles on
// a TTY the helper runs the first-login prompt, defaults an empty entry to the
// current directory basename, and signals newProfile=true.
func TestResolveLoginTargetFirstLoginDefault(t *testing.T) {
	seedGhProfiles(t) // zero profiles
	chdirClean(t)
	stubInteractive(t, true)
	pwd, _ := os.Getwd()
	want := profile.DefaultName("", pwd)
	c, out := pickCmd("\n") // empty entry -> default
	name, _, newProfile, err := resolveLoginTargetWithProfiles(c, github.NewProvider(), nil, "ghrl", validGhName)
	if err != nil {
		t.Fatalf("first-login should not error: %v", err)
	}
	if !newProfile {
		t.Fatal("first-login should signal newProfile=true")
	}
	if name != want {
		t.Fatalf("empty entry should use dir default %q, got %q", want, name)
	}
	if !strings.Contains(out.String(), "No ghrl profiles yet") || !strings.Contains(out.String(), "["+want+"]") {
		t.Fatalf("prompt should show dir default: %q", out.String())
	}
}

// TestResolveLoginTargetFirstLoginTyped proves a typed name overrides the default.
func TestResolveLoginTargetFirstLoginTyped(t *testing.T) {
	seedGhProfiles(t)
	chdirClean(t)
	stubInteractive(t, true)
	c, _ := pickCmd("typedname\n")
	name, _, newProfile, err := resolveLoginTargetWithProfiles(c, github.NewProvider(), nil, "ghrl", validGhName)
	if err != nil || !newProfile || name != "typedname" {
		t.Fatalf("typed name should win: name=%q newProfile=%v err=%v", name, newProfile, err)
	}
}

// TestResolveLoginTargetFirstLoginReprompt proves an invalid name re-prompts and a
// subsequent valid one is accepted.
func TestResolveLoginTargetFirstLoginReprompt(t *testing.T) {
	seedGhProfiles(t)
	chdirClean(t)
	stubInteractive(t, true)
	c, out := pickCmd("bad/name\ngood\n")
	name, _, newProfile, err := resolveLoginTargetWithProfiles(c, github.NewProvider(), nil, "ghrl", validGhName)
	if err != nil || !newProfile || name != "good" {
		t.Fatalf("invalid-then-valid: name=%q newProfile=%v err=%v", name, newProfile, err)
	}
	if !strings.Contains(out.String(), "invalid profile name") {
		t.Fatalf("should report the invalid name: %q", out.String())
	}
}

// TestResolveLoginTargetFirstLoginNonInteractive proves the zero-profile
// non-interactive path is unchanged (friendly error, newProfile=false).
func TestResolveLoginTargetFirstLoginNonInteractive(t *testing.T) {
	seedGhProfiles(t)
	chdirClean(t)
	stubInteractive(t, false)
	c, _ := pickCmd("")
	_, _, newProfile, err := resolveLoginTargetWithProfiles(c, github.NewProvider(), nil, "ghrl", validGhName)
	if err == nil || !strings.Contains(err.Error(), "no profiles yet") {
		t.Fatalf("want no-profiles error, got %v", err)
	}
	if newProfile {
		t.Fatal("non-interactive must not signal newProfile")
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
	got, _, err := resolveLoginTarget(c, aws.NewProvider(), nil, "azrl aws", validAwsName)
	if err != nil || got != "prod" {
		t.Fatalf("got %q err %v", got, err)
	}
	if !strings.Contains(out.String(), "azrl aws: using the only profile") {
		t.Fatalf("missing announce: %q", out.String())
	}
}
