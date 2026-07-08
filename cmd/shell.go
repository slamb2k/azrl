package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/gcp"
	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// shellEnv builds the env pairs a subshell needs to act as the profile — the
// same values the .envrc writers emit — plus the AZRL_PROFILE marker and the
// profile's mapped browser command (so git push / az login inside the
// subshell route to the right browser profile).
func shellEnv(providerName, name string) ([]string, error) {
	var env []string
	browser := ""
	switch providerName {
	case "azure":
		dir := config.ProfilesDir()
		c, err := profile.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown azure profile %q: %w", name, err)
		}
		env = append(env, "AZURE_CONFIG_DIR="+filepath.Join(dir, name))
		browser = c.BrowserCmd
	case "github":
		dir := config.GithubProfilesDir()
		c, err := github.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown github profile %q: %w", name, err)
		}
		env = append(env, "GH_CONFIG_DIR="+github.ConfigDir(dir, name))
		browser = c.BrowserCmd
	case "aws":
		dir := config.AwsProfilesDir()
		c, err := aws.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown aws profile %q: %w", name, err)
		}
		if c.Isolate {
			env = append(env,
				"AWS_CONFIG_FILE="+filepath.Join(dir, name, "config"),
				"AWS_SHARED_CREDENTIALS_FILE="+filepath.Join(dir, name, "credentials"))
		} else {
			env = append(env, "AWS_PROFILE="+name)
		}
		browser = c.BrowserCmd
	case "gcp":
		dir := config.GcpProfilesDir()
		c, err := gcp.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown gcp profile %q: %w", name, err)
		}
		if c.Isolate {
			env = append(env, "CLOUDSDK_CONFIG="+filepath.Join(dir, name))
		}
		env = append(env, "CLOUDSDK_ACTIVE_CONFIG_NAME="+c.ResolvedConfigName(name))
		browser = c.BrowserCmd
	default:
		return nil, fmt.Errorf("azrl: unknown provider %q", providerName)
	}
	if browser != "" {
		env = append(env, "AZRL_BROWSER_CMD="+browser)
	}
	env = append(env, "AZRL_PROFILE="+providerName+":"+name)
	return env, nil
}

// shellLoginRunner and shellExit are test seams: login-first execs the real
// azrl login as a child, and exit-status passthrough must not kill the test
// binary.
var (
	shellLoginRunner = runShellLogin
	shellExit        = os.Exit
)

// runShellLogin signs the profile in by exec-ing the real login verb as a
// child (bridge, browser mapping, everything — CLI-first). The promoted ghrl
// binary has github verbs at top level, so the gh group prefix is dropped
// there, mirroring internal/ui's groupArgs.
func runShellLogin(providerName, name string) error {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "azrl"
	}
	var args []string
	switch providerName {
	case "azure":
		args = []string{"login", name}
	case "github":
		if strings.TrimSuffix(filepath.Base(self), ".exe") == "ghrl" {
			args = []string{"login", name}
		} else {
			args = []string{"gh", "login", name}
		}
	default:
		args = []string{providerName, "login", name}
	}
	c := exec.Command(self, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// shellNeedsLogin reports whether the profile's cached session is unusable —
// disk-only, via the provider registry's Status.
func shellNeedsLogin(providerName, name string) bool {
	for _, p := range provider.All() {
		if p.Name() != providerName {
			continue
		}
		st, err := p.Status(name, p.ProfilesDir())
		return err != nil || !provider.SessionLive(st)
	}
	return true
}

// shellOwnedKeys are the vars a provider's subshell owns outright: any value
// inherited from an outer azrl shell would silently outrank the inner
// profile (innermost wins), so runShell scrubs them before applying its own.
func shellOwnedKeys(providerName string) []string {
	keys := []string{"AZRL_BROWSER_CMD", "AZRL_PROFILE"}
	switch providerName {
	case "azure":
		keys = append(keys, "AZURE_CONFIG_DIR")
	case "github":
		keys = append(keys, "GH_CONFIG_DIR")
	case "aws":
		keys = append(keys, "AWS_PROFILE", "AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE")
	case "gcp":
		keys = append(keys, "CLOUDSDK_CONFIG", "CLOUDSDK_ACTIVE_CONFIG_NAME")
	}
	return keys
}

// scrubEnv drops any entries in base whose key matches one of keys, so the
// caller's own values (appended after) can't be shadowed by a stale
// inherited one of the same key that survives Go's last-wins de-dup.
func scrubEnv(base []string, keys []string) []string {
	out := base[:0:0]
	for _, kv := range base {
		drop := false
		for _, k := range keys {
			if strings.HasPrefix(kv, k+"=") {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

// runShell drops the user into $SHELL acting as the profile: sign in first if
// the session is dead, then exec the shell with the profile's env map. No
// directory link is read or written.
func runShell(providerName, name string, out io.Writer) error {
	env, err := shellEnv(providerName, name)
	if err != nil {
		return err
	}
	if shellNeedsLogin(providerName, name) {
		fmt.Fprintf(out, "azrl: no live session for %s:%s — signing in first\n", providerName, name)
		if err := shellLoginRunner(providerName, name); err != nil {
			return fmt.Errorf("azrl: sign-in failed — not starting a shell: %w", err)
		}
	}
	if cur := os.Getenv("AZRL_PROFILE"); cur != "" {
		fmt.Fprintf(out, "azrl: already inside an azrl shell (%s) — nesting; innermost wins\n", cur)
	}
	fmt.Fprintf(out, "azrl: shell as %s (%s) — 'exit' returns\n", name, providerName)
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	c := exec.Command(sh)
	c.Env = append(scrubEnv(os.Environ(), shellOwnedKeys(providerName)), env...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			shellExit(ee.ExitCode())
			return nil
		}
		return fmt.Errorf("azrl: shell failed to start: %w", err)
	}
	return nil
}

func newShellCmd(providerName, short string) *cobra.Command {
	return &cobra.Command{
		Use:          "shell <name>",
		Short:        short,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShell(providerName, args[0], cmd.OutOrStdout())
		},
	}
}

func init() {
	RootCmd.AddCommand(newShellCmd("azure", "Open a subshell acting as an Azure profile (no mapping)"))
}
