package profile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnvrcContent is the direnv stanza that pins AZURE_CONFIG_DIR to this dir's
// profile by reading .azprofile, so plain `az` follows azrl in this tree.
// It is guarded: when the pointer is gone (unmapped later) or names a
// missing profile dir the stanza is inert and az falls back to ambient —
// the unguarded original silently steered az at a wrong/empty config dir.
const EnvrcContent = `azrl_profile="$(cat .azprofile 2>/dev/null || true)"
if [ -n "$azrl_profile" ] && [ -d "$HOME/.azure-profiles/$azrl_profile" ]; then
  export AZURE_CONFIG_DIR="$HOME/.azure-profiles/$azrl_profile"
fi
unset azrl_profile
`

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

// EnvrcWarning returns a caution when dir's .envrc still steers the named
// provider's CLI after its mapping was removed — direnv keeps exporting the
// stanza until the file is updated. "" when there is no .envrc concern.
func EnvrcWarning(providerName, dir string) string {
	markers := map[string][]string{
		"azure": {"AZURE_CONFIG_DIR"},
		"aws":   {"AWS_PROFILE", "AWS_CONFIG_FILE"},
		"gcp":   {"CLOUDSDK"},
	}[providerName]
	if len(markers) == 0 {
		return ""
	}
	b, err := os.ReadFile(EnvrcPath(dir))
	if err != nil {
		return ""
	}
	for _, m := range markers {
		if strings.Contains(string(b), m) {
			return "note: " + EnvrcPath(dir) + " still exports this provider's env — remove or update it"
		}
	}
	return ""
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
