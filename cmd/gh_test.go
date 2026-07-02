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

func TestGhSwitchRecordsCurrent(t *testing.T) {
	home := seedGhHome(t)
	runRoot(t, "gh", "switch", "work")
	b, err := os.ReadFile(filepath.Join(home, ".github-profiles", ".current"))
	if err != nil || strings.TrimSpace(string(b)) != "work" {
		t.Fatalf(".current=%q err=%v", string(b), err)
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
