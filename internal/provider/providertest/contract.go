// Package providertest holds the shared Provider contract suite. Every provider
// (Azure, GitHub, …) runs RunContract from its own test to guarantee identical
// profile-mechanic behaviour, which is what lets the CLI and tabbed TUI drive
// them through one code path.
package providertest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slamb2k/azrl/internal/provider"
)

// RunContract exercises the provider-agnostic surface: identity metadata plus a
// full pin → resolve → list → relabel → remove round-trip against temp dirs.
func RunContract(t *testing.T, p provider.Provider) {
	t.Helper()

	if p.Name() == "" {
		t.Error("Name() is empty")
	}
	if p.Title() == "" {
		t.Error("Title() is empty")
	}
	if p.ProfilesDir() == "" {
		t.Error("ProfilesDir() is empty")
	}

	s := p.Scheme()
	if s.Pointer == "" || s.DetailKey == "" || s.LabelKey == "" {
		t.Fatalf("Scheme is under-specified: %+v", s)
	}

	confdir := t.TempDir()
	pwd := t.TempDir()
	conf := filepath.Join(confdir, "acme.conf")
	if err := os.WriteFile(conf, []byte(s.DetailKey+"=example.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pin the repo to the profile and resolve it back.
	if err := p.Use("acme", confdir, pwd); err != nil {
		t.Fatalf("Use: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(pwd, s.Pointer)); err != nil || string(b) != "acme\n" {
		t.Fatalf("pointer %s not written: %q err=%v", s.Pointer, string(b), err)
	}
	got, err := p.Resolve("", pwd)
	if err != nil || got != "acme" {
		t.Fatalf("Resolve: got %q err=%v", got, err)
	}

	// Listing surfaces the profile with its detail; the reserved conf is excluded.
	if err := os.WriteFile(filepath.Join(confdir, s.Reserved+".conf"), []byte("X=y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profs, err := p.ListProfiles(confdir)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(profs) != 1 || profs[0].Name != "acme" {
		t.Fatalf("ListProfiles: %+v", profs)
	}
	if profs[0].Detail != "example.test" {
		t.Fatalf("detail from %s: %+v", s.DetailKey, profs[0])
	}

	// Relabel changes the display name but not the slug.
	if err := p.SetLabel("acme", confdir, "Acme Co"); err != nil {
		t.Fatalf("SetLabel: %v", err)
	}
	profs, _ = p.ListProfiles(confdir)
	if profs[0].Display() != "Acme Co" || profs[0].Name != "acme" {
		t.Fatalf("relabel: %+v", profs[0])
	}

	// Status is a disk-only snapshot; it must never shell out to a provider CLI.
	sentinel := shimNoNetwork(t)
	if err := p.Scheme().Touch("acme", confdir, pwd); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	st, err := p.Status("acme", confdir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.ProfileName != "acme" {
		t.Fatalf("Status.ProfileName = %q", st.ProfileName)
	}
	if st.LastUsed.IsZero() {
		t.Fatal("Status.LastUsed not populated after Touch")
	}
	if st.Directory != pwd {
		t.Fatalf("Status.Directory = %q, want %q", st.Directory, pwd)
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("Status shelled out to a provider CLI (network call)")
	}

	// WatchDirs is a best-effort disk enumeration for the dashboard's fs watcher:
	// sanity-call it to confirm it never panics (it may legitimately be empty).
	_ = p.WatchDirs()

	// Removing an unknown profile via Use must error.
	if err := p.Use("ghost", confdir, t.TempDir()); err == nil {
		t.Fatal("Use of unknown profile should error")
	}

	// Remove cleans up the conf and the matching pointer.
	removed, err := p.Remove("acme", confdir, pwd)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(removed) < 2 {
		t.Fatalf("Remove targets: %+v", removed)
	}
	if _, err := os.Stat(conf); !os.IsNotExist(err) {
		t.Fatal("conf not removed")
	}
}

// shimNoNetwork installs az/gh/aws/gcloud fakes on PATH that touch a sentinel
// and exit 1, so a Status() that shells out is caught. Returns the sentinel path.
func shimNoNetwork(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	sentinel := filepath.Join(bin, "invoked")
	for _, name := range []string{"az", "gh", "aws", "gcloud"} {
		script := "#!/usr/bin/env bash\ntouch \"" + sentinel + "\"\nexit 1\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return sentinel
}
