package github

import (
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// WatchDirs returns the existing dirs Status() reads: the profiles root and each
// per-profile isolated GH_CONFIG_DIR (holding hosts.yml). Best-effort; only
// existing dirs are returned.
func (Provider) WatchDirs() []string {
	return provider.ChildDirs(config.GithubProfilesDir())
}
