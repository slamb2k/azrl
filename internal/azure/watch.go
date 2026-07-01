package azure

import (
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// WatchDirs returns the existing dirs Status() reads: the profiles root and each
// per-profile isolated AZURE_CONFIG_DIR (holding the MSAL cache), plus the
// default ~/.azure. Best-effort; only existing dirs are returned.
func (Provider) WatchDirs() []string {
	dirs := provider.ChildDirs(config.ProfilesDir())
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".azure"))
	}
	return provider.ExistingDirs(dirs)
}
