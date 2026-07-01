package aws_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slamb2k/azrl/internal/aws"
)

func TestWatchDirsReturnsExistingDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	profiles := filepath.Join(home, ".aws-profiles")
	iso := filepath.Join(profiles, "acme")
	ssoCache := filepath.Join(home, ".aws", "sso", "cache")
	os.MkdirAll(iso, 0o755)
	os.MkdirAll(ssoCache, 0o755)

	dirs := aws.NewProvider().WatchDirs()

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
	if !has(ssoCache) {
		t.Fatalf("WatchDirs missing SSO cache dir %q: %v", ssoCache, dirs)
	}
	// A profile with no isolated dir on disk must be stat-filtered out.
	ghost := filepath.Join(profiles, "ghost")
	for _, d := range dirs {
		if d == ghost {
			t.Fatalf("WatchDirs returned a non-existent dir: %q", d)
		}
	}
}
