package gcp

import (
	"fmt"
	"os"

	"github.com/slamb2k/azrl/internal/profile"
)

// EnvrcContent is the direnv stanza that makes plain `gcloud` in this dir follow
// the profile. By default it exports CLOUDSDK_ACTIVE_CONFIG_NAME (selecting the
// named configuration); under isolation it points CLOUDSDK_CONFIG at the
// profile's own config dir.
func EnvrcContent(name string, isolate bool) string {
	if isolate {
		return fmt.Sprintf("export CLOUDSDK_CONFIG=\"$HOME/.gcp-profiles/%s\"\n", name)
	}
	return fmt.Sprintf("export CLOUDSDK_ACTIVE_CONFIG_NAME=%s\n", name)
}

// WriteEnvrc writes the stanza into pwd, refusing to clobber an existing .envrc.
// It reports whether it wrote one.
func WriteEnvrc(pwd, name string, isolate bool) (bool, error) {
	if profile.HasEnvrc(pwd) {
		return false, nil
	}
	if err := os.WriteFile(profile.EnvrcPath(pwd), []byte(EnvrcContent(name, isolate)), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
