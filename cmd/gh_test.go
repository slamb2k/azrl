package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestGhRmRemovesProfile(t *testing.T) {
	home := seedGhHome(t)
	runRoot(t, "gh", "rm", "work")
	if _, err := os.Stat(filepath.Join(home, ".github-profiles", "work.conf")); !os.IsNotExist(err) {
		t.Fatal("work.conf not removed")
	}
}
