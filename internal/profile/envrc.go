package profile

import (
	"os"
	"path/filepath"
)

// EnvrcContent is the direnv stanza that pins AZURE_CONFIG_DIR to this dir's
// profile by reading .azprofile, so plain `az` follows azrl in this tree.
const EnvrcContent = `export AZURE_CONFIG_DIR="$HOME/.azure-profiles/$(cat .azprofile)"` + "\n"

// EnvrcPath returns the .envrc path for pwd.
func EnvrcPath(pwd string) string { return filepath.Join(pwd, ".envrc") }

// HasEnvrc reports whether pwd already has an .envrc.
func HasEnvrc(pwd string) bool {
	_, err := os.Stat(EnvrcPath(pwd))
	return err == nil
}

// WriteEnvrc writes the stanza into pwd, refusing to clobber an existing file.
// It reports whether it wrote one.
func WriteEnvrc(pwd string) (bool, error) {
	p := EnvrcPath(pwd)
	if _, err := os.Stat(p); err == nil {
		return false, nil
	}
	if err := os.WriteFile(p, []byte(EnvrcContent), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
