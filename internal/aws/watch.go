package aws

import (
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// WatchDirs returns the existing dirs Status() reads: the profiles root, each
// per-profile isolated dir, and the shared SSO token cache (~/.aws/sso/cache).
// Best-effort; only existing dirs are returned.
func (Provider) WatchDirs() []string {
	dirs := provider.ChildDirs(config.AwsProfilesDir())
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".aws", "sso", "cache"))
	}
	return provider.ExistingDirs(dirs)
}
