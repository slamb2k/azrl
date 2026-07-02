package github_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slamb2k/azrl/internal/github"
)

func TestWatchDirsReturnsExistingDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	profiles := filepath.Join(home, ".github-profiles")
	iso := filepath.Join(profiles, "work")
	os.MkdirAll(iso, 0o755)

	dirs := github.NewProvider().WatchDirs()

	has := func(want string) bool {
		for _, d := range dirs {
			if d == want {
				return true
			}
		}
		return false
	}
	if !has(profiles) {
		t.Fatalf("WatchDirs missing profiles root %q: %v", profiles, dirs)
	}
	// The per-profile isolated GH_CONFIG_DIR (holding hosts.yml) is watched.
	if !has(iso) {
		t.Fatalf("WatchDirs missing isolated dir %q: %v", iso, dirs)
	}
	// A profile with no isolated dir on disk must be stat-filtered out.
	ghost := filepath.Join(profiles, "ghost")
	for _, d := range dirs {
		if d == ghost {
			t.Fatalf("WatchDirs returned a non-existent dir: %q", d)
		}
	}
}

// TestWatchDirsIncludesAmbientConfigDir proves the native hosts.yml dir
// (${GH_CONFIG_DIR:-~/.config/gh}) is watched so ambient rows live-update.
func TestWatchDirsIncludesAmbientConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	native := filepath.Join(home, ".config", "gh")
	os.MkdirAll(native, 0o755)
	t.Setenv("GH_CONFIG_DIR", "")

	found := false
	for _, d := range github.NewProvider().WatchDirs() {
		if d == native {
			found = true
		}
	}
	if !found {
		t.Fatalf("WatchDirs missing native config dir %q", native)
	}

	// GH_CONFIG_DIR wins over the ~/.config/gh default.
	override := filepath.Join(home, "custom-gh")
	os.MkdirAll(override, 0o755)
	t.Setenv("GH_CONFIG_DIR", override)
	found = false
	for _, d := range github.NewProvider().WatchDirs() {
		if d == override {
			found = true
		}
	}
	if !found {
		t.Fatalf("WatchDirs missing GH_CONFIG_DIR %q", override)
	}
}
