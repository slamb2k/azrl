package gcp

import (
	"os"
	"path/filepath"
	"strings"
)

// driftedDefault reports whether the cwd-pinned profile's gcloud configuration
// differs from the ambient active configuration. gcloud selects a config by the
// CLOUDSDK_ACTIVE_CONFIG_NAME env var, falling back to the plain-text
// active_config file in ~/.config/gcloud, defaulting to "default". Only the
// cwd-pinned profile can drift.
func driftedDefault(name, confdir, configName string) bool {
	pwd, err := os.Getwd()
	if err != nil {
		return false
	}
	pinned, _ := scheme.Resolve("", pwd)
	if pinned != name {
		return false
	}
	return ambientActiveConfig(gcloudConfigDir(name, confdir, false)) != configName
}

// ambientActiveConfig resolves the gcloud active configuration name from the
// env var, else the active_config file under gcloudDir, else "default".
func ambientActiveConfig(gcloudDir string) string {
	if v := os.Getenv("CLOUDSDK_ACTIVE_CONFIG_NAME"); v != "" {
		return v
	}
	if b, err := os.ReadFile(filepath.Join(gcloudDir, "active_config")); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	return "default"
}

// driftedIsolate reports whether an isolated profile's ambient session differs
// from the profile pinned by the cwd. Only the cwd-pinned profile can drift:
// when the pointer here names name but CLOUDSDK_CONFIG doesn't point at name's
// isolated dir (including when it's unset), the shell would act as a different
// configuration than this dir pins.
func driftedIsolate(name, confdir string) bool {
	pwd, err := os.Getwd()
	if err != nil {
		return false
	}
	pinned, _ := scheme.Resolve("", pwd)
	if pinned != name {
		return false
	}
	return os.Getenv("CLOUDSDK_CONFIG") != filepath.Join(confdir, name)
}
