package profile

import (
	"os"
	"os/exec"
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

// DirenvAllow trusts pwd's .envrc via `direnv allow` so the stanza loads on the
// shell's next prompt. It reports whether direnv was found and run; a missing
// direnv is not an error (the caller falls back to a manual hint).
func DirenvAllow(pwd string) (ran bool, err error) {
	bin, lookErr := exec.LookPath("direnv")
	if lookErr != nil {
		return false, nil
	}
	cmd := exec.Command(bin, "allow", pwd)
	cmd.Dir = pwd
	return true, cmd.Run()
}
