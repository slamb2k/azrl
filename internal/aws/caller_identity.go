package aws

import (
	"os"
	"os/exec"
	"path/filepath"
)

// CallerIdentity shells `aws sts get-caller-identity --profile <name> --output
// json` and returns its stdout. With isolate set it scopes
// AWS_CONFIG_FILE/AWS_SHARED_CREDENTIALS_FILE to the profile's own files, the
// same way Login and SyncConfig do, so the identity check reads the isolated
// session rather than the shared ~/.aws files.
func CallerIdentity(dir, name string, isolate bool) ([]byte, error) {
	cmd := exec.Command("aws", "sts", "get-caller-identity", "--profile", name, "--output", "json")
	env := os.Environ()
	if isolate {
		env = append(env,
			"AWS_CONFIG_FILE="+filepath.Join(dir, name, "config"),
			"AWS_SHARED_CREDENTIALS_FILE="+filepath.Join(dir, name, "credentials"),
		)
	}
	cmd.Env = env
	return cmd.Output()
}
