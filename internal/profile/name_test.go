package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"work", "acme-prod", "client_a.dev", "Mixed.Case-1", "a..b"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", ".hidden", "../x", "a/b", `a\b`, "a\nb", "a\tb", "\x00"} {
		if err := ValidateName(bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("ValidateName(%q) = %v, want ErrInvalidName", bad, err)
		}
	}
}

// TestResolveRejectsHostilePointer proves a pointer file shipped by a cloned
// repo cannot steer a profile name outside the profiles dir.
func TestResolveRejectsHostilePointer(t *testing.T) {
	for _, body := range []string{"..\n", "../../../../proc/self/cwd/evil\n", ".x\n"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".ghprofile"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if got, err := ghScheme.Resolve("", dir); !errors.Is(err, ErrInvalidName) || got != "" {
			t.Fatalf("Resolve with pointer %q = (%q, %v), want ErrInvalidName", body, got, err)
		}
	}
	if _, err := ghScheme.Resolve("..", t.TempDir()); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("explicit .. must be rejected, got %v", err)
	}
}

// TestRemoveNeverEscapesConfdir is the regression for `rm ..` deleting the
// profiles dir's parent ($HOME).
func TestRemoveNeverEscapesConfdir(t *testing.T) {
	home := t.TempDir()
	confdir := filepath.Join(home, ".github-profiles")
	if err := os.MkdirAll(confdir, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(home, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"..", "."} {
		if got := ghScheme.RemoveTargets(name, confdir, home); got != nil {
			t.Fatalf("RemoveTargets(%q) = %v, want none", name, got)
		}
		if _, err := ghScheme.Remove(name, confdir, home); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("Remove(%q) err = %v, want ErrInvalidName", name, err)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("home contents must survive")
	}
	if _, err := os.Stat(confdir); err != nil {
		t.Fatal("profiles dir must survive")
	}
}
