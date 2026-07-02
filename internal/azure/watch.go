package azure

import (
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// WatchDirs returns the existing dirs Status() and Ambient() read: the profiles
// root, each per-profile isolated AZURE_CONFIG_DIR (holding the MSAL cache),
// and the native ${AZURE_CONFIG_DIR:-~/.azure} (holding azureProfile.json).
// Best-effort; only existing dirs are returned.
func (Provider) WatchDirs() []string {
	dirs := provider.ChildDirs(config.ProfilesDir())
	if d, _, ok := provider.EnvOrHome("AZURE_CONFIG_DIR", ".azure"); ok {
		dirs = append(dirs, d)
	}
	return provider.ExistingDirs(dirs)
}
