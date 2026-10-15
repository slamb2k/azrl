package github

import (
	"fmt"
	"os"

	"github.com/slamb2k/azrl/internal/profile"
)

// EnvrcContent is the direnv stanza that makes plain `gh` in this dir follow
// the profile by pointing GH_CONFIG_DIR at its isolated config dir. GitHub
// profiles are always isolated, so isolate is accepted only to share the
// WriteEnvrc signature with the other providers.
func EnvrcContent(name string) string {
	return fmt.Sprintf("export GH_CONFIG_DIR=\"$HOME/.github-profiles/%s\"\n", name)
}

// WriteEnvrc writes the stanza into pwd, refusing to clobber an existing .envrc.
// It reports whether it wrote one.
func WriteEnvrc(pwd, name string, _ bool) (bool, error) {
	if profile.HasEnvrc(pwd) {
		return false, nil
	}
	if err := os.WriteFile(profile.EnvrcPath(pwd), []byte(EnvrcContent(name)), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
