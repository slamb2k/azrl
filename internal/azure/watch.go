package azure

import (
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// WatchDirs returns the existing dirs Status() and Ambient() read: the profiles
// root, each per-profile isolated AZURE_CONFIG_DIR (holding the MSAL cache),
// and the native ${AZURE_CONFIG_DIR:-~/.azure} (holding azureProfile.json).
// Best-effort; only existing dirs are returned.
func (Provider) WatchDirs() []string {
	dirs := provider.ChildDirs(config.ProfilesDir())
	if d := os.Getenv("AZURE_CONFIG_DIR"); d != "" {
		dirs = append(dirs, d)
	} else if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".azure"))
	}
	return provider.ExistingDirs(dirs)
}
