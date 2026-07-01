package github

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// WhoAmI returns the login of the account signed into the profile's isolated
// GH_CONFIG_DIR, via `gh api user` scoped to that dir and host.
func WhoAmI(profilesDir, name, host string) (string, error) {
	dir := ConfigDir(profilesDir, name)
	cmd := exec.Command("gh", "api", "user", "--hostname", host)
	cmd.Env = append(os.Environ(), "GH_CONFIG_DIR="+dir)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ghrl: gh api user failed for %q: %w", name, err)
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(out, &u); err != nil {
		return "", fmt.Errorf("ghrl: could not parse gh api user for %q: %w", name, err)
	}
	return u.Login, nil
}

// AssertAccount verifies the profile's isolated GH_CONFIG_DIR is authenticated
// and, when GH_USER is set, that the signed-in login matches it.
func AssertAccount(profilesDir, name string, c Conf) error {
	login, err := WhoAmI(profilesDir, name, c.Host)
	if err != nil {
		return err
	}
	if c.User != "" && c.User != login {
		return fmt.Errorf("ghrl: USER MISMATCH — expected %q, got %q", c.User, login)
	}
	return nil
}
