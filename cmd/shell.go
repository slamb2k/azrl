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
			return nil, profileLoadErr(err, "azrl: unknown azure profile %q: %w", name, err)
		}
		env = append(env, "AZURE_CONFIG_DIR="+filepath.Join(dir, name))
		browser = c.BrowserCmd
	case "github":
		dir := config.GithubProfilesDir()
		c, err := github.LoadConf(name, dir)
		if err != nil {
			return nil, profileLoadErr(err, "azrl: unknown github profile %q: %w", name, err)
		}
		env = append(env, "GH_CONFIG_DIR="+github.ConfigDir(dir, name))
		browser = c.BrowserCmd
	case "aws":
		dir := config.AwsProfilesDir()
		c, err := aws.LoadConf(name, dir)
		if err != nil {
			return nil, profileLoadErr(err, "azrl: unknown aws profile %q: %w", name, err)
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
			return nil, profileLoadErr(err, "azrl: unknown gcp profile %q: %w", name, err)
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

// loginArgs is the CLI argv (sans binary) that signs providerName's profile
// in. The promoted ghrl binary has github verbs at top level, so the gh group
// prefix is dropped there, mirroring internal/ui's groupArgs.
func loginArgs(self, providerName, name string) []string {
	switch providerName {
	case "azure":
		return []string{"login", name}
	case "github":
		if strings.TrimSuffix(filepath.Base(self), ".exe") == "ghrl" {
			return []string{"login", name}
		}
		return []string{"gh", "login", name}
	default:
		return []string{providerName, "login", name}
	}
}

// selfExe is the running binary's path, falling back to "azrl".
func selfExe() string {
	self, err := os.Executable()
	if err != nil || self == "" {
		return "azrl"
	}
	return self
}

// runShellLogin signs the profile in by exec-ing the real login verb as a
// child (bridge, browser mapping, everything — CLI-first).
func runShellLogin(providerName, name string) error {
	self := selfExe()
	c := exec.Command(self, loginArgs(self, providerName, name)...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// loginHint is the command a human should run to sign the profile in, for
// needs-human messages.
func loginHint(providerName, name string) string {
	self := selfExe()
	bin := strings.TrimSuffix(filepath.Base(self), ".exe")
	if bin != "ghrl" {
		bin = "azrl"
	}
	return bin + " " + strings.Join(loginArgs(self, providerName, name), " ")
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
	if err := ensureSession(providerName, name, true, out); err != nil {
		return err
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

// ensureSession signs a dead session in before a shell or command runs. With
// mayLogin it execs the real login; otherwise it refuses with ExitNeedsHuman
// and the exact command for a human to run — so an unattended `shell -- cmd`
// (an agent) never triggers a browser sign-in.
func ensureSession(providerName, name string, mayLogin bool, out io.Writer) error {
	if !shellNeedsLogin(providerName, name) {
		return nil
	}
	if !mayLogin {
		return needsHumanErr("azrl: %s:%s has no live session and needs an interactive sign-in — ask the user to run: %s",
			providerName, name, loginHint(providerName, name))
	}
	fmt.Fprintf(out, "azrl: no live session for %s:%s — signing in first\n", providerName, name)
	if err := shellLoginRunner(providerName, name); err != nil {
		return fmt.Errorf("azrl: sign-in failed — not starting: %w", err)
	}
	return nil
}

// shellCommandRunner is a test seam over running one command as the profile.
var shellCommandRunner = runCommandAs

// runCommandAs runs argv once with the profile's environment (the same map
// `shell` gives a subshell) and passes its exit status through.
func runCommandAs(providerName, name string, argv []string, out io.Writer) error {
	env, err := shellEnv(providerName, name)
	if err != nil {
		return err
	}
	if err := ensureSession(providerName, name, isInteractive(), out); err != nil {
		return err
	}
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return withCode(127, fmt.Errorf("azrl: %s: command not found", argv[0]))
	}
	c := exec.Command(bin, argv[1:]...)
	c.Env = append(scrubEnv(os.Environ(), shellOwnedKeys(providerName)), env...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			shellExit(ee.ExitCode())
			return nil
		}
		return fmt.Errorf("azrl: %s failed to start: %w", argv[0], err)
	}
	return nil
}

func newShellCmd(providerName, short string) *cobra.Command {
	return &cobra.Command{
		Use:   "shell [name] [-- <command> [args…]]",
		Short: short,
		Long: short + ".\n\n" +
			"With a command after --, runs just that command as the profile and exits with its status\n" +
			"(e.g. `shell work -- az group list`). Without a TTY a dead session is never signed in:\n" +
			"it exits 3 with the login command for a human to run.",
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nameArgs, run := args, []string(nil)
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				nameArgs, run = args[:dash], args[dash:]
				if len(run) == 0 {
					return usageErr("azrl: nothing to run after --")
				}
			}
			if len(nameArgs) > 1 {
				return usageErr("azrl: shell takes at most one profile name before --")
			}
			name, err := nameOrGoverning(providerName, nameArgs)
			if err != nil {
				return err
			}
			if run != nil {
				return shellCommandRunner(providerName, name, run, cmd.ErrOrStderr())
			}
			return runShell(providerName, name, cmd.ErrOrStderr())
		},
	}
}
