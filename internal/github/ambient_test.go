package github_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/provider"
)

func writeHosts(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAmbientLegacySingleUserShape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", "")
	writeHosts(t, filepath.Join(home, ".config", "gh"),
		"github.com:\n    user: alice\n    oauth_token: t1\n    git_protocol: https\n")

	a, err := github.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "alice@github.com" {
		t.Fatalf("Identity = %q", a.Identity)
	}
	if a.Source != "file:~/.config/gh/hosts.yml" {
		t.Fatalf("Source = %q", a.Source)
	}
}

func TestAmbientModernMultiAccountShapePicksActiveUser(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", "")
	writeHosts(t, filepath.Join(home, ".config", "gh"),
		"github.com:\n"+
			"    users:\n"+
			"        alice:\n"+
			"            oauth_token: t1\n"+
			"        bob:\n"+
			"            oauth_token: t2\n"+
			"    user: bob\n"+
			"    oauth_token: t2\n"+
			"    git_protocol: https\n")

	a, err := github.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "bob@github.com" {
		t.Fatalf("Identity = %q, want the host's active user bob@github.com", a.Identity)
	}
}

func TestAmbientUsersMapOnlySingleAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", "")
	writeHosts(t, filepath.Join(home, ".config", "gh"),
		"github.com:\n"+
			"    users:\n"+
			"        alice:\n"+
			"            oauth_token: t1\n"+
			"    oauth_token: t1\n")

	a, err := github.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "alice@github.com" {
		t.Fatalf("Identity = %q", a.Identity)
	}
}

func TestAmbientHonorsGhConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir)
	writeHosts(t, dir, "ghe.example.com:\n    user: carol\n    oauth_token: t3\n")

	a, err := github.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "carol@ghe.example.com" {
		t.Fatalf("Identity = %q", a.Identity)
	}
	if a.Source != "file:$GH_CONFIG_DIR/hosts.yml" {
		t.Fatalf("Source = %q", a.Source)
	}
}

func TestAmbientZeroOnMissingOrUnknownShape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", "")

	a, err := github.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a != (provider.Ambient{}) {
		t.Fatalf("missing hosts.yml: got %+v, want zero", a)
	}

	writeHosts(t, filepath.Join(home, ".config", "gh"), "- just\n- a list\n")
	a, err = github.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a != (provider.Ambient{}) {
		t.Fatalf("unknown shape: got %+v, want zero", a)
	}
}
