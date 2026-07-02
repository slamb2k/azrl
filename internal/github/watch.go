package github

import (
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// WatchDirs returns the existing dirs Status() and Ambient() read: the profiles
// root, each per-profile isolated GH_CONFIG_DIR (holding hosts.yml), and the
// native ${GH_CONFIG_DIR:-~/.config/gh}. Best-effort; only existing dirs are
// returned.
func (Provider) WatchDirs() []string {
	dirs := provider.ChildDirs(config.GithubProfilesDir())
	if d := os.Getenv("GH_CONFIG_DIR"); d != "" {
		dirs = append(dirs, d)
	} else if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "gh"))
	}
	return provider.ExistingDirs(dirs)
}
