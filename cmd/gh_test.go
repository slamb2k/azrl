package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slamb2k/azrl/internal/profile"
)

// seedGhHome points HOME at a temp dir with two GitHub profiles on disk.
func seedGhHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	gp := filepath.Join(home, ".github-profiles")
	os.MkdirAll(gp, 0o755)
	os.WriteFile(filepath.Join(gp, "work.conf"), []byte("GH_HOST=github.com\nGH_USER=octocat\n"), 0o644)
	os.WriteFile(filepath.Join(gp, "emu.conf"), []byte("GH_HOST=acme.ghe.com\nGH_USER=alice\n"), 0o644)
	seedGlobalConf(t, home)
	return home
}

// installFakeGhGit puts passing gh and git shims on PATH.
func installFakeGhGit(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	for _, n := range []string{"gh", "git"} {
		os.WriteFile(filepath.Join(bin, n), []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func runRoot(t *testing.T, args ...string) string {
	t.Helper()
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs(args)
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v (out=%q)", args, err, buf.String())
	}
	return buf.String()
}

func TestGhListPrintsProfiles(t *testing.T) {
	seedGhHome(t)
	out := runRoot(t, "gh", "list")
	if !strings.Contains(out, "work") || !strings.Contains(out, "github.com") ||
		!strings.Contains(out, "emu") || !strings.Contains(out, "acme.ghe.com") {
		t.Fatalf("gh list output:\n%s", out)
	}
}

// TestGhSwitchCommandRemoved proves the `switch` command is fully gone from
// both the gh group and the promoted ghrl top level: unknown command, no
// .current file written, absent from help output.
func TestGhSwitchCommandRemoved(t *testing.T) {
	home := seedGhHome(t)

	if c, _, _ := RootCmd.Find([]string{"gh", "switch"}); c != nil && c.Name() == "switch" {
		t.Fatal("switch must not resolve to a command")
	}
	if _, statErr := os.Stat(filepath.Join(home, ".github-profiles", ".current")); !os.IsNotExist(statErr) {
		t.Fatal("removed switch must not write .current")
	}

	ghrl := GhrlRoot()
	buf := new(bytes.Buffer)
	ghrl.SetOut(buf)
	ghrl.SetErr(buf)
	if c, _, _ := ghrl.Find([]string{"switch"}); c != nil && c.Name() == "switch" {
		t.Fatal("ghrl switch must not resolve to a command")
	}
	_ = buf

	if help := runRoot(t, "gh", "--help"); strings.Contains(help, "switch") {
		t.Fatalf("switch must be hidden from gh help:\n%s", help)
	}
	ghrlHelp := GhrlRoot()
	hbuf := new(bytes.Buffer)
	ghrlHelp.SetOut(hbuf)
	ghrlHelp.SetErr(hbuf)
	ghrlHelp.SetArgs([]string{"--help"})
	if err := ghrlHelp.Execute(); err != nil {
		t.Fatalf("ghrl --help: %v", err)
	}
	if strings.Contains(hbuf.String(), "switch") {
		t.Fatalf("switch must be hidden from ghrl help:\n%s", hbuf.String())
	}
}

// TestGhStatusIgnoresStaleCurrent proves a leftover .current file changes
// nothing: gh status neither reads nor mentions it.
func TestGhStatusIgnoresStaleCurrent(t *testing.T) {
	home := seedGhHome(t)
	chdirClean(t)
	os.WriteFile(filepath.Join(home, ".github-profiles", ".current"), []byte("work\n"), 0o644)

	out := runRoot(t, "gh", "status")
	if strings.Contains(out, "active profile") || strings.Contains(out, "work") {
		t.Fatalf("stale .current must be ignored:\n%s", out)
	}
	if !strings.Contains(out, "no .ghprofile pin") {
		t.Fatalf("status should still report the pin state:\n%s", out)
	}
}

func TestGhUsePinsRepo(t *testing.T) {
	seedGhHome(t)
	installFakeGhGit(t)
	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	runRoot(t, "gh", "use", "work")
	b, err := os.ReadFile(filepath.Join(work, ".ghprofile"))
	if err != nil || strings.TrimSpace(string(b)) != "work" {
		t.Fatalf(".ghprofile=%q err=%v", string(b), err)
	}
}

func TestGhLoginCreatesWithYes(t *testing.T) {
	home := seedGhHome(t)
	installFakeGhGit(t)
	chdirClean(t)

	out := runRoot(t, "gh", "login", "fresh", "--yes")
	if !strings.Contains(out, `created profile "fresh" (github.com)`) {
		t.Fatalf("missing created-profile announce:\n%s", out)
	}
	b, err := os.ReadFile(filepath.Join(home, ".github-profiles", "fresh.conf"))
	if err != nil {
		t.Fatalf("fresh.conf not written: %v", err)
	}
	if !strings.Contains(string(b), "GH_HOST=github.com") {
		t.Fatalf("created conf missing host:\n%s", b)
	}
	// Pin-on-create: the new profile pins the cwd.
	pwd, _ := os.Getwd()
	if pin, err := os.ReadFile(filepath.Join(pwd, ".ghprofile")); err != nil || strings.TrimSpace(string(pin)) != "fresh" {
		t.Fatalf(".ghprofile not pinned on create (err=%v pin=%q)", err, pin)
	}
}

// TestGhLoginFirstLoginCreatesFromPrompt proves that on a TTY with zero saved
// profiles, `gh login` (no name) prompts for a name (defaulting to the dir),
// creates the profile and signs in — with no second [y/N] confirm.
func TestGhLoginFirstLoginCreatesFromPrompt(t *testing.T) {
	home := seedGhProfiles(t) // zero profiles
	installFakeGhGit(t)
	chdirClean(t)
	stubInteractive(t, true)
	pwd, _ := os.Getwd()
	want := profile.DefaultName("", pwd)

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetIn(strings.NewReader("\n")) // accept the dir default
	RootCmd.SetArgs([]string{"gh", "login"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("first-login should succeed: %v (out=%q)", err, buf.String())
	}
	if !strings.Contains(buf.String(), "No ghrl profiles yet") {
		t.Fatalf("missing first-login prompt:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `created profile "`+want+`"`) {
		t.Fatalf("missing created-profile announce:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "[y/N]") {
		t.Fatalf("must not double-confirm the just-named profile:\n%s", buf.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".github-profiles", want+".conf")); err != nil {
		t.Fatalf("profile conf not created: %v", err)
	}
}

func TestGhLoginUnknownNonInteractiveErrors(t *testing.T) {
	home := seedGhHome(t)
	installFakeGhGit(t)
	chdirClean(t)
	stubInteractive(t, false)

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	// --yes=false is explicit so a prior test's persisted flag can't leak in.
	RootCmd.SetArgs([]string{"gh", "login", "ghost", "--yes=false"})
	err := RootCmd.Execute()
	if err == nil {
		t.Fatalf("unknown profile non-interactive should error (out=%q)", buf.String())
	}
	if !strings.Contains(err.Error(), `no profile "ghost"`) {
		t.Fatalf("wrong error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".github-profiles", "ghost.conf")); !os.IsNotExist(statErr) {
		t.Fatal("no conf should be written when creation is declined")
	}
	if strings.Contains(buf.String(), "Usage:") {
		t.Fatalf("runtime error must not dump usage:\n%s", buf.String())
	}
}

func TestGhRmRemovesProfile(t *testing.T) {
	home := seedGhHome(t)
	runRoot(t, "gh", "rm", "work")
	if _, err := os.Stat(filepath.Join(home, ".github-profiles", "work.conf")); !os.IsNotExist(err) {
		t.Fatal("work.conf not removed")
	}
}

// fakeGhWhoAmI installs a gh shim answering `gh api user --hostname ...` with
// the given login, for capture's WhoAmI call.
func fakeGhWhoAmI(t *testing.T, login string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/usr/bin/env bash\nprintf '{\"login\":\"" + login + "\"}'\n"
	os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// fakeGhIsolatedFailsAmbientSucceeds installs a gh shim that fails whoami
// when GH_CONFIG_DIR is set (simulating an isolated profile dir with no
// session yet) and succeeds with the given login when it's unset (the
// ambient/global gh session) — for exercising capture's fallback narrowing.
func fakeGhIsolatedFailsAmbientSucceeds(t *testing.T, ambientLogin string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/usr/bin/env bash\n" +
		"if [ -n \"$GH_CONFIG_DIR\" ]; then echo no session >&2; exit 1; fi\n" +
		"printf '{\"login\":\"" + ambientLogin + "\"}'\n"
	os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestGhCaptureFallsBackToAmbientWhenNoSession proves a fresh adopt (isolated
// GH_CONFIG_DIR has no hosts.yml yet) still falls back to the ambient
// identity when WhoAmI fails — the pre-existing "adopt" behavior.
func TestGhCaptureFallsBackToAmbientWhenNoSession(t *testing.T) {
	home := seedGhHome(t)
	fakeGhIsolatedFailsAmbientSucceeds(t, "ambient-alice")

	runRoot(t, "gh", "capture", "work")

	gp := filepath.Join(home, ".github-profiles")
	b, err := os.ReadFile(filepath.Join(gp, "work.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "GH_USER=ambient-alice") {
		t.Fatalf("fresh adopt should record the ambient identity:\n%s", string(b))
	}
}

// TestGhCaptureDoesNotFallBackWhenSessionExists proves a profile whose
// isolated GH_CONFIG_DIR already holds a session (hosts.yml present) does
// NOT fall back to the ambient identity on a WhoAmI error — regression for
// the fallback silently overwriting GH_USER from an unrelated ambient
// account on a transient failure.
func TestGhCaptureDoesNotFallBackWhenSessionExists(t *testing.T) {
	home := seedGhHome(t)
	fakeGhIsolatedFailsAmbientSucceeds(t, "ambient-mallory")

	gp := filepath.Join(home, ".github-profiles")
	os.MkdirAll(filepath.Join(gp, "work"), 0o755)
	os.WriteFile(filepath.Join(gp, "work", "hosts.yml"), []byte("github.com:\n"), 0o644)

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"gh", "capture", "work"})
	err := RootCmd.Execute()
	if err == nil {
		t.Fatal("capture should surface the WhoAmI error when a session already exists, not fall back")
	}
	if !strings.Contains(err.Error(), "gh api user failed") {
		t.Fatalf("wrong error: %v", err)
	}

	b, readErr := os.ReadFile(filepath.Join(gp, "work.conf"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(b), "ambient-mallory") {
		t.Fatalf("conf must not be overwritten with the ambient identity:\n%s", string(b))
	}
	if !strings.Contains(string(b), "GH_USER=octocat") {
		t.Fatalf("conf's original GH_USER must be untouched:\n%s", string(b))
	}
}

func TestGhCapturePreservesExistingKeys(t *testing.T) {
	home := seedGhHome(t)
	gp := filepath.Join(home, ".github-profiles")
	os.WriteFile(filepath.Join(gp, "work.conf"),
		[]byte("GH_HOST=github.com\nGH_USER=octocat\nGH_LABEL=Keep Me\nGH_BROWSER_CMD=chrome-work\nGH_BROWSER_LABEL=Edge — Work\n"), 0o644)
	fakeGhWhoAmI(t, "octocat")

	runRoot(t, "gh", "capture", "work")

	b, err := os.ReadFile(filepath.Join(gp, "work.conf"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, "GH_LABEL=Keep Me") || !strings.Contains(out, "GH_BROWSER_CMD=chrome-work") ||
		!strings.Contains(out, "GH_BROWSER_LABEL=Edge — Work") {
		t.Fatalf("recapture wiped existing keys:\n%s", out)
	}
}

// TestGhLoginNoLinkSkipsPin proves created-but-no-map skips prov.Use: the
// conf is written but no .ghprofile pin lands in pwd. Mirrors
// TestGhLoginCreatesWithYes's seeding/shims.
func TestGhLoginNoLinkSkipsPin(t *testing.T) {
	home := seedGhHome(t)
	installFakeGhGit(t)
	chdirClean(t)
	pwd, _ := os.Getwd()
	// Shared RootCmd flag (same leak resetAwsCaptureFlags guards against):
	// Flags().Set writes through cobra's BoolVar binding to the closure var.
	t.Cleanup(func() {
		c, _, err := RootCmd.Find([]string{"gh", "login"})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Flags().Set("no-map", "false"); err != nil {
			t.Fatal(err)
		}
	})

	out := runRoot(t, "gh", "login", "neu", "--yes", "--no-map")
	if !strings.Contains(out, `created profile "neu" (github.com)`) {
		t.Fatalf("missing created-profile announce:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".github-profiles", "neu.conf")); err != nil {
		t.Fatalf("neu.conf not written: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(pwd, ".ghprofile")); !os.IsNotExist(statErr) {
		t.Fatal("--no-map must not pin .ghprofile")
	}
}
