package gcp

import (
	"os"
	"path/filepath"
)

// gcloudConfigDir returns the gcloud config directory a profile reads/writes:
// the isolated <confdir>/<name> under isolate, else the ambient CLOUDSDK_CONFIG
// override, else the default ~/.config/gcloud.
func gcloudConfigDir(name, confdir string, isolate bool) string {
	if isolate {
		return filepath.Join(confdir, name)
	}
	if v := os.Getenv("CLOUDSDK_CONFIG"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gcloud")
}

// selectorEnv scopes a gcloud invocation to a profile's resolved configuration:
// under isolate it overrides the whole config dir via CLOUDSDK_CONFIG (keyed by
// the profile name); in every case it selects the named configuration via
// CLOUDSDK_ACTIVE_CONFIG_NAME=configName — the same name SyncConfig binds under —
// so an isolated dir created with --no-activate is still selected correctly.
func selectorEnv(dir, name, configName string, isolate bool) []string {
	env := os.Environ()
	if isolate {
		env = append(env, "CLOUDSDK_CONFIG="+filepath.Join(dir, name))
	}
	return append(env, "CLOUDSDK_ACTIVE_CONFIG_NAME="+configName)
}
