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
