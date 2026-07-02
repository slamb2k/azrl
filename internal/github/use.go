package github

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/slamb2k/azrl/internal/profile"
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
		_ = profile.RecordMapping(profilesDir, profile.Mapping{Dir: pwd, Profile: name, Source: "gitconfig"})
	}
	return nil
}
