package aws

import (
	"fmt"
	"os"

	"github.com/slamb2k/azrl/internal/profile"
)

// EnvrcContent is the direnv stanza that makes plain `aws` in this dir follow the
// profile. By default it exports AWS_PROFILE (the ambient-profile method); under
// isolation it instead points AWS_CONFIG_FILE/AWS_SHARED_CREDENTIALS_FILE at the
// profile's own files.
func EnvrcContent(name string, isolate bool) string {
	if isolate {
		return fmt.Sprintf("export AWS_CONFIG_FILE=\"$HOME/.aws-profiles/%s/config\"\nexport AWS_SHARED_CREDENTIALS_FILE=\"$HOME/.aws-profiles/%s/credentials\"\n", name, name)
	}
	return fmt.Sprintf("export AWS_PROFILE=%s\n", name)
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
