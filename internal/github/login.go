package github

import (
	"os"
	"os/exec"
)

// browserCommand returns the BROWSER value gh should use to open the device
// activation page: the GHRL_BROWSER override if set, else the running binary
// invoked as its hidden __browser shim. gh honors $BROWSER, so the local browser
// pops via the shim's relay.
func browserCommand() string {
	if c := os.Getenv("GHRL_BROWSER"); c != "" {
		return c
	}
	self, err := os.Executable()
	if err != nil {
		self = "azrl"
	}
	return self + " __browser"
}

// Login runs `gh auth login` for the profile with an isolated GH_CONFIG_DIR and
// --insecure-storage, forcing the token into the per-profile hosts.yml (the gh
// keyring is global and would otherwise collide across profiles). The web flow's
// device page opens on the local machine via the BROWSER shim. gh's own prompts
// run on the user's terminal.
func Login(profilesDir, name string, c Conf) error {
	dir := ConfigDir(profilesDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	protocol := c.Protocol
	if protocol == "" {
		protocol = "https"
	}
	args := []string{"auth", "login", "--hostname", c.Host,
		"--git-protocol", protocol, "--insecure-storage", "--web"}
	cmd := exec.Command("gh", args...)
	cmd.Env = append(os.Environ(),
		"GH_CONFIG_DIR="+dir,
		"BROWSER="+browserCommand(),
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
