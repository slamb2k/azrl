package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUse(t *testing.T) {
	confdir := t.TempDir()
	work := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	if err := Use("acme", confdir, work); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(work, ".azprofile"))
	if string(b) != "acme\n" {
		t.Fatalf("azprofile=%q", string(b))
	}
	if err := Use("ghost", confdir, t.TempDir()); err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestRemoveTargetsAndRemove(t *testing.T) {
	confdir := t.TempDir()
	work := t.TempDir()
	os.MkdirAll(filepath.Join(confdir, "acme"), 0o755)
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	os.WriteFile(filepath.Join(work, ".azprofile"), []byte("acme\n"), 0o644)
	if got := RemoveTargets("acme", confdir, work); len(got) != 3 {
		t.Fatalf("want 3 targets, got %v", got)
	}
	if _, err := Remove("acme", confdir, work); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(confdir, "acme.conf")); !os.IsNotExist(err) {
		t.Fatal("conf not removed")
	}
	if _, err := os.Stat(filepath.Join(work, ".azprofile")); !os.IsNotExist(err) {
		t.Fatal("azprofile not removed")
	}
}

func TestRemoveLeavesNonMatchingAzprofile(t *testing.T) {
	confdir := t.TempDir()
	work := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	os.WriteFile(filepath.Join(work, ".azprofile"), []byte("other\n"), 0o644)
	got := RemoveTargets("acme", confdir, work)
	for _, p := range got {
		if filepath.Base(p) == ".azprofile" {
			t.Fatal("non-matching .azprofile must not be a target")
		}
	}
}
