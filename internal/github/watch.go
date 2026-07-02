package github

import (
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// WatchDirs returns the existing dirs Status() and Ambient() read: the profiles
// root, each per-profile isolated GH_CONFIG_DIR (holding hosts.yml), and the
// native ${GH_CONFIG_DIR:-~/.config/gh}. Best-effort; only existing dirs are
// returned.
func (Provider) WatchDirs() []string {
	dirs := provider.ChildDirs(config.GithubProfilesDir())
	if d, _, ok := provider.EnvOrHome("GH_CONFIG_DIR", ".config", "gh"); ok {
		dirs = append(dirs, d)
	}
	return provider.ExistingDirs(dirs)
}
