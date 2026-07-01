package github

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// AssertAccount verifies the profile's isolated GH_CONFIG_DIR is authenticated
// and, when GH_USER is set, that the signed-in login matches it. It runs
// `gh api user` scoped to the profile's config dir and host.
func AssertAccount(profilesDir, name string, c Conf) error {
	dir := ConfigDir(profilesDir, name)
	cmd := exec.Command("gh", "api", "user", "--hostname", c.Host)
	cmd.Env = append(os.Environ(), "GH_CONFIG_DIR="+dir)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ghrl: gh api user failed for %q: %w", name, err)
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(out, &u); err != nil {
		return fmt.Errorf("ghrl: could not parse gh api user for %q: %w", name, err)
	}
	if c.User != "" && c.User != u.Login {
		return fmt.Errorf("ghrl: USER MISMATCH — expected %q, got %q", c.User, u.Login)
	}
	return nil
}
