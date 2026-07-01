package github

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SetupRepo wires git-HTTPS credentials for a pinned repo: it registers gh as
// the credential helper for the host (scoped to the profile's GH_CONFIG_DIR) and
// sets the repo-local credential username so pushes resolve to this account's
// token even when two accounts share a host (GCM's multi-account method).
func SetupRepo(profilesDir, name, pwd string, c Conf) error {
	dir := ConfigDir(profilesDir, name)
	setup := exec.Command("gh", "auth", "setup-git", "--hostname", c.Host)
	setup.Env = append(os.Environ(), "GH_CONFIG_DIR="+dir)
	if err := setup.Run(); err != nil {
		return fmt.Errorf("ghrl: gh auth setup-git failed: %w", err)
	}
	if c.User != "" {
		key := fmt.Sprintf("credential.https://%s.username", c.Host)
		if err := exec.Command("git", "-C", pwd, "config", "--local", key, c.User).Run(); err != nil {
			return fmt.Errorf("ghrl: setting %s failed: %w", key, err)
		}
	}
	return nil
}

// Switch records name as the active profile in <profilesDir>/.current after
// verifying its conf exists. This is the global default used when a repo has no
// .ghprofile pin.
func Switch(profilesDir, name string) error {
	conf := filepath.Join(profilesDir, name+".conf")
	if _, err := os.Stat(conf); err != nil {
		return fmt.Errorf("ghrl: no such profile %q (missing %s)", name, conf)
	}
	return os.WriteFile(filepath.Join(profilesDir, ".current"), []byte(name+"\n"), 0o644)
}

// Current returns the active profile recorded by Switch, or "" when none is set.
func Current(profilesDir string) string {
	b, err := os.ReadFile(filepath.Join(profilesDir, ".current"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
