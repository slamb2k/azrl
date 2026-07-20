package github

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/slamb2k/azrl/internal/profile"
)

// repoRoot resolves the enclosing repo's toplevel so the recorded mapping
// matches what the read path (rev-parse in the overview) resolves; falls back
// to dir outside a repo.
func repoRoot(dir string) string {
	out, err := GitCmd(dir, "rev-parse", "--show-toplevel").Output()
	if root := strings.TrimSpace(string(out)); err == nil && root != "" {
		return root
	}
	return dir
}

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
		if err := GitCmd(pwd, "config", "--local", key, c.User).Run(); err != nil {
			return fmt.Errorf("ghrl: setting %s failed: %w", key, err)
		}
		_ = profile.RecordMapping(profilesDir, profile.Mapping{Dir: repoRoot(pwd), Profile: name, Source: "gitconfig"})
	}
	return nil
}
