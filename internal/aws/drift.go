package aws

import (
	"os"
	"path/filepath"
)

// driftedIsolate reports whether an isolated profile's ambient session differs
// from the profile pinned by the cwd. Only the cwd-pinned profile can drift:
// when the pointer here names name but AWS_CONFIG_FILE/AWS_SHARED_CREDENTIALS_FILE
// don't both point at name's isolated files (including when either is unset), the
// shell would act as a different profile than this dir pins.
func driftedIsolate(name, confdir string) bool {
	pwd, err := os.Getwd()
	if err != nil {
		return false
	}
	pinned, _ := scheme.Resolve("", pwd)
	if pinned != name {
		return false
	}
	cfg := filepath.Join(confdir, name, "config")
	creds := filepath.Join(confdir, name, "credentials")
	return !(os.Getenv("AWS_CONFIG_FILE") == cfg && os.Getenv("AWS_SHARED_CREDENTIALS_FILE") == creds)
}
