package gcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slamb2k/azrl/internal/gcp"
)

func TestWatchDirsReturnsExistingDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	profiles := filepath.Join(home, ".gcp-profiles")
	iso := filepath.Join(profiles, "acme")
	gcloud := filepath.Join(home, ".config", "gcloud")
	configs := filepath.Join(gcloud, "configurations")
	os.MkdirAll(iso, 0o755)
	os.MkdirAll(configs, 0o755)

	dirs := gcp.NewProvider().WatchDirs()

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
	if !has(iso) {
		t.Fatalf("WatchDirs missing isolated dir %q: %v", iso, dirs)
	}
	if !has(gcloud) {
		t.Fatalf("WatchDirs missing gcloud dir %q: %v", gcloud, dirs)
	}
	if !has(configs) {
		t.Fatalf("WatchDirs missing configurations dir %q: %v", configs, dirs)
	}
	// A profile with no isolated dir on disk must be stat-filtered out.
	ghost := filepath.Join(profiles, "ghost")
	for _, d := range dirs {
		if d == ghost {
			t.Fatalf("WatchDirs returned a non-existent dir: %q", d)
		}
	}
}

// TestWatchDirsHonoursCloudsdkConfig proves CLOUDSDK_CONFIG overrides the
// ~/.config/gcloud default so ambient rows live-update from the right dir.
func TestWatchDirsHonoursCloudsdkConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	override := filepath.Join(home, "custom-gcloud")
	os.MkdirAll(filepath.Join(override, "configurations"), 0o755)
	t.Setenv("CLOUDSDK_CONFIG", override)

	var haveDir, haveConfigs bool
	for _, d := range gcp.NewProvider().WatchDirs() {
		if d == override {
			haveDir = true
		}
		if d == filepath.Join(override, "configurations") {
			haveConfigs = true
		}
	}
	if !haveDir || !haveConfigs {
		t.Fatalf("WatchDirs missing CLOUDSDK_CONFIG dirs under %q", override)
	}
}
