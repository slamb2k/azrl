package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ghScheme is a GitHub-flavored Scheme used to prove that the profile
// mechanics are parameterized (pointer filename, reserved conf name, detail
// and label keys) rather than hardcoded to Azure.
var ghScheme = Scheme{
	Pointer:   ".ghprofile",
	Reserved:  "ghrl",
	DetailKey: "GH_HOST",
	LabelKey:  "GH_LABEL",
	Prefix:    "ghrl",
}

func TestSchemeResolveExplicitAndWalkUp(t *testing.T) {
	if got, _ := ghScheme.Resolve("work", "/tmp"); got != "work" {
		t.Fatalf("explicit: %q", got)
	}
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ghprofile"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ghScheme.Resolve("", deep)
	if err != nil || got != "work" {
		t.Fatalf("walk-up: got %q err %v", got, err)
	}
	if _, err := ghScheme.Resolve("", t.TempDir()); err == nil {
		t.Fatal("expected error when no .ghprofile")
	}
}

func TestSchemeUseAndRemove(t *testing.T) {
	confdir := t.TempDir()
	work := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "work.conf"), []byte("GH_HOST=github.com\n"), 0o644)
	if err := ghScheme.Use("work", confdir, work); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(work, ".ghprofile"))
	if string(b) != "work\n" {
		t.Fatalf("ghprofile=%q", string(b))
	}
	if err := ghScheme.Use("ghost", confdir, t.TempDir()); err == nil {
		t.Fatal("expected error for unknown profile")
	}
	os.MkdirAll(filepath.Join(confdir, "work"), 0o755)
	got := ghScheme.RemoveTargets("work", confdir, work)
	if len(got) != 3 {
		t.Fatalf("want 3 targets, got %v", got)
	}
}

func TestSchemeListExcludesReserved(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "work.conf"), []byte("GH_HOST=github.com\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "emu.conf"), []byte("GH_HOST=acme.ghe.com\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "ghrl.conf"), []byte("LOCAL_HOST=x\n"), 0o644)
	got, err := ghScheme.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 profiles, got %d: %+v", len(got), got)
	}
	for _, p := range got {
		if p.Name == "ghrl" {
			t.Fatal("ghrl.conf must be excluded")
		}
		if p.Name == "work" && p.Detail != "github.com" {
			t.Fatalf("detail from GH_HOST: %+v", p)
		}
	}
}

func TestSchemeTouchAndLastTouch(t *testing.T) {
	confdir := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "work.conf"), []byte("GH_HOST=github.com\nGH_USER=octocat\n"), 0o644)
	if lu, dir := ghScheme.LastTouch("work", confdir); !lu.IsZero() || dir != "" {
		t.Fatalf("untouched profile: lu=%v dir=%q", lu, dir)
	}
	bound := t.TempDir()
	if err := ghScheme.Touch("work", confdir, bound); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(confdir, "work.conf"))
	for _, want := range []string{"GH_HOST=github.com", "GH_USER=octocat", "LAST_USED=", "LAST_DIR=" + bound} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("conf missing %q:\n%s", want, b)
		}
	}
	lu, dir := ghScheme.LastTouch("work", confdir)
	if lu.IsZero() {
		t.Fatal("LAST_USED not read back")
	}
	if dir != bound {
		t.Fatalf("LAST_DIR = %q, want %q", dir, bound)
	}
	// A second touch updates in place, not appending duplicate keys.
	next := t.TempDir()
	if err := ghScheme.Touch("work", confdir, next); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(confdir, "work.conf"))
	if strings.Count(string(b), "LAST_DIR=") != 1 || strings.Count(string(b), "LAST_USED=") != 1 {
		t.Fatalf("touch duplicated keys:\n%s", b)
	}
	if _, dir := ghScheme.LastTouch("work", confdir); dir != next {
		t.Fatalf("LAST_DIR not updated: %q", dir)
	}
}

func TestSchemeSetLabelPreservesKeys(t *testing.T) {
	confdir := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "work.conf"), []byte("GH_HOST=github.com\nGH_USER=octocat\n"), 0o644)
	if err := ghScheme.SetLabel("work", confdir, "Work Account"); err != nil {
		t.Fatal(err)
	}
	profs, _ := ghScheme.List(confdir)
	if len(profs) != 1 || profs[0].Display() != "Work Account" || profs[0].Name != "work" {
		t.Fatalf("display/slug: %+v", profs)
	}
	if profs[0].Detail != "github.com" {
		t.Fatalf("relabel clobbered GH_HOST: %+v", profs[0])
	}
	if err := ghScheme.SetLabel("work", confdir, ""); err != nil {
		t.Fatal(err)
	}
	profs, _ = ghScheme.List(confdir)
	if profs[0].Display() != "work" {
		t.Fatalf("empty label should revert to slug: %+v", profs[0])
	}
}
