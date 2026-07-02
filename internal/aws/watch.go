package aws

import (
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// WatchDirs returns the existing dirs Status() and Ambient() read: the profiles
// root, each per-profile isolated dir, the shared SSO token cache
// (~/.aws/sso/cache), and the dir of the native ${AWS_CONFIG_FILE:-~/.aws/config}.
// Best-effort; only existing dirs are returned.
func (Provider) WatchDirs() []string {
	dirs := provider.ChildDirs(config.AwsProfilesDir())
	if f, _, ok := provider.EnvOrHome("AWS_CONFIG_FILE", ".aws", "config"); ok {
		dirs = append(dirs, filepath.Dir(f))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".aws", "sso", "cache"))
	}
	return provider.ExistingDirs(dirs)
}
