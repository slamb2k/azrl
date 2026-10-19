package github

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/slamb2k/azrl/internal/bridge"
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

// CredentialHelper is the git credential helper SetupRepo installs: gh's own
// git-credential, pinned to the profile's isolated GH_CONFIG_DIR so the repo
// authenticates as this profile whether or not direnv has exported anything.
func CredentialHelper(ghPath, configDir string) string {
	return fmt.Sprintf("!GH_CONFIG_DIR=%s %s auth git-credential", bridge.ShellQuote(configDir), bridge.ShellQuote(ghPath))
}

// SetupRepo wires git-HTTPS credentials for a pinned repo, entirely in the
// repo's own .git/config — the user's global git config (and any helper it
// names, e.g. Git Credential Manager) is never touched. It resets the host's
// inherited helpers for this repo (the empty entry), registers gh pinned to
// the profile's GH_CONFIG_DIR, and sets the credential username so pushes
// resolve to this account's token even when two accounts share a host.
func SetupRepo(profilesDir, name, pwd string, c Conf) error {
	dir := ConfigDir(profilesDir, name)
	gh, err := exec.LookPath("gh")
	if err != nil {
		return fmt.Errorf("ghrl: gh not found on PATH: %w", err)
	}
	helperKey := fmt.Sprintf("credential.https://%s.helper", c.Host)
	if err := GitCmd(pwd, "config", "--local", "--replace-all", helperKey, "").Run(); err != nil {
		return fmt.Errorf("ghrl: setting %s failed (not a git repo?): %w", helperKey, err)
	}
	if err := GitCmd(pwd, "config", "--local", "--add", helperKey, CredentialHelper(gh, dir)).Run(); err != nil {
		return fmt.Errorf("ghrl: setting %s failed: %w", helperKey, err)
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
