package gcp

import (
	"path/filepath"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/provider"
)

// WatchDirs returns the existing dirs Status() and Ambient() read: the profiles
// root, each per-profile isolated CLOUDSDK_CONFIG dir, and the native
// ${CLOUDSDK_CONFIG:-~/.config/gcloud} dir (plus its configurations/ subdir
// holding active_config and config_<name>). Best-effort; only existing dirs
// are returned.
func (Provider) WatchDirs() []string {
	dirs := provider.ChildDirs(config.GcpProfilesDir())
	if gc, _, ok := provider.EnvOrHome("CLOUDSDK_CONFIG", ".config", "gcloud"); ok {
		dirs = append(dirs, gc, filepath.Join(gc, "configurations"))
	}
	return provider.ExistingDirs(dirs)
}
