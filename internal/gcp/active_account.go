package gcp

import (
	"os/exec"
	"strings"
)

// ActiveAccount shells `gcloud auth list --filter=status:ACTIVE
// --format=value(account)` scoped to a profile's resolved configuration (via
// CLOUDSDK_ACTIVE_CONFIG_NAME=configName, plus CLOUDSDK_CONFIG under isolate) and
// returns the signed-in account email. This is the one on-demand network/CLI
// identity check; Status stays disk-only.
func ActiveAccount(dir, name, configName string, isolate bool) (string, error) {
	cmd := exec.Command("gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=value(account)")
	cmd.Env = selectorEnv(dir, name, configName, isolate)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
