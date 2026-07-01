package gcp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/slamb2k/azrl/internal/config"
)

// SyncConfig idempotently ensures the profile's gcloud named configuration
// exists (create-if-absent, never clobbering an existing one) and binds its
// project/compute-region. Under isolate the configuration lives in the profile's
// own CLOUDSDK_CONFIG dir. It is the GCP analogue of the AWS SyncConfig.
func SyncConfig(name string, c Conf, isolate bool) error {
	configName := c.ResolvedConfigName(name)
	dir := config.GcpProfilesDir()
	env := os.Environ()
	if isolate {
		env = append(env, "CLOUDSDK_CONFIG="+filepath.Join(dir, name))
	}

	// Create the configuration; ignore the "already exists" case so re-runs are
	// idempotent and never clobber an existing named configuration. --no-activate
	// keeps gcloud from flipping the machine's global active configuration as a
	// side effect (gcloud activates newly created configs by default).
	create := exec.Command("gcloud", "config", "configurations", "create", configName, "--no-activate")
	create.Env = env
	if out, err := create.CombinedOutput(); err != nil {
		if !strings.Contains(string(out), "already exists") {
			return fmt.Errorf("gcp: create configuration %q: %w: %s", configName, err, strings.TrimSpace(string(out)))
		}
	}

	if c.Project != "" {
		set := exec.Command("gcloud", "config", "set", "project", c.Project, "--configuration", configName)
		set.Env = env
		if out, err := set.CombinedOutput(); err != nil {
			return fmt.Errorf("gcp: set project: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	if c.Region != "" {
		set := exec.Command("gcloud", "config", "set", "compute/region", c.Region, "--configuration", configName)
		set.Env = env
		if out, err := set.CombinedOutput(); err != nil {
			return fmt.Errorf("gcp: set compute/region: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
