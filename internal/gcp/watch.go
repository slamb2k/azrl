package gcp

import (
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// WatchDirs returns the existing dirs Status() reads: the profiles root, each
// per-profile isolated CLOUDSDK_CONFIG dir, and the shared ~/.config/gcloud dir
// (plus its configurations/ subdir holding active_config and config_<name>).
// Best-effort; only existing dirs are returned.
func (Provider) WatchDirs() []string {
	dirs := provider.ChildDirs(config.GcpProfilesDir())
	if home, err := os.UserHomeDir(); err == nil {
		gc := filepath.Join(home, ".config", "gcloud")
		dirs = append(dirs, gc, filepath.Join(gc, "configurations"))
	}
	return provider.ExistingDirs(dirs)
}
