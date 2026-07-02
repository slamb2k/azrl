package azure_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slamb2k/azrl/internal/azure"
)

func TestWatchDirsReturnsExistingDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	profiles := filepath.Join(home, ".azure-profiles")
	iso := filepath.Join(profiles, "acme")
	os.MkdirAll(iso, 0o755)

	dirs := azure.NewProvider().WatchDirs()

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
	// ~/.azure does not exist here, so it must be stat-filtered out.
	for _, d := range dirs {
		if d == filepath.Join(home, ".azure") {
			t.Fatalf("WatchDirs returned a non-existent dir: %q", d)
		}
	}
}

// TestWatchDirsIncludesAmbientConfigDir proves the native azureProfile.json
// dir (${AZURE_CONFIG_DIR:-~/.azure}) is watched so ambient rows live-update.
func TestWatchDirsIncludesAmbientConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	native := filepath.Join(home, ".azure")
	os.MkdirAll(native, 0o755)
	t.Setenv("AZURE_CONFIG_DIR", "")

	found := false
	for _, d := range azure.NewProvider().WatchDirs() {
		if d == native {
			found = true
		}
	}
	if !found {
		t.Fatalf("WatchDirs missing native config dir %q", native)
	}

	// AZURE_CONFIG_DIR wins over the ~/.azure default.
	override := filepath.Join(home, "custom-azure")
	os.MkdirAll(override, 0o755)
	t.Setenv("AZURE_CONFIG_DIR", override)
	found = false
	for _, d := range azure.NewProvider().WatchDirs() {
		if d == override {
			found = true
		}
	}
	if !found {
		t.Fatalf("WatchDirs missing AZURE_CONFIG_DIR %q", override)
	}
}
