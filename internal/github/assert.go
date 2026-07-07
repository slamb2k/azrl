package github

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

func WhoAmI(profilesDir, name, host string) (string, error) {
	login, err := whoAmI(host, ConfigDir(profilesDir, name))
	if err != nil {
		return "", fmt.Errorf("ghrl: gh api user failed for %q: %w", name, err)
	}
	return login, nil
}

// AmbientWhoAmI returns the login gh's own ambient config is signed in as —
// the capture fallback when the profile's isolated GH_CONFIG_DIR has no
// session yet (adopting the native default identity).
func AmbientWhoAmI(host string) (string, error) {
	login, err := whoAmI(host, "")
	if err != nil {
		return "", fmt.Errorf("ghrl: gh api user failed for the ambient session: %w", err)
	}
	return login, nil
}

func whoAmI(host, configDir string) (string, error) {
	cmd := exec.Command("gh", "api", "user", "--hostname", host)
	cmd.Env = os.Environ()
	if configDir != "" {
		cmd.Env = append(cmd.Env, "GH_CONFIG_DIR="+configDir)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(out, &u); err != nil {
		return "", fmt.Errorf("could not parse gh api user: %w", err)
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
