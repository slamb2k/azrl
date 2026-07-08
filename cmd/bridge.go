package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// bridgeShimScript is the single-word $BROWSER target `azrl bridge` points a
// child command at. A bare executable path is the one convention every
// browser launcher handles (python's webbrowser needs `%s` for arguments and
// `&` to avoid blocking; gh shell-splits; GCM ignores $BROWSER entirely) —
// and the shim backgrounds the real bridge and returns immediately, so
// launchers that wait for $BROWSER to exit never block the OAuth callback
// they are about to serve.
func bridgeShimScript(bin string) string {
	return fmt.Sprintf("#!/bin/sh\nnohup %q __browser \"$1\" >/dev/null 2>&1 &\n", bin)
}

// writeBridgeShim writes the shim into a private temp dir, returning its
// path and a cleanup func. The dir can be removed as soon as the bridged
// command exits — the backgrounded __browser process no longer needs it.
func writeBridgeShim() (string, func(), error) {
	dir, err := os.MkdirTemp("", "azrl-bridge-")
	if err != nil {
		return "", nil, err
	}
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "azrl"
	}
	p := filepath.Join(dir, "azrl-browser")
	if err := os.WriteFile(p, []byte(bridgeShimScript(self)), 0o755); err != nil {
		os.RemoveAll(dir)
		return "", nil, err
	}
	return p, func() { os.RemoveAll(dir) }, nil
}

// bridgeRun execs the command with $BROWSER wired to the shim, streaming its
// stdio. It returns the child's exit code (0 on success) so callers can react
// (e.g. gh default's switch→login fallback); a non-ExitError failure to start
// is the returned error. azrl never touches where the command stores its
// session — this is plumbing only (PAT-002).
func bridgeRun(args []string, out io.Writer) (int, error) {
	shim, cleanup, err := writeBridgeShim()
	if err != nil {
		return 0, fmt.Errorf("azrl: bridge: %w", err)
	}
	defer cleanup()
	c := exec.Command(args[0], args[1:]...)
	c.Env = append(scrubEnv(os.Environ(), []string{"BROWSER"}), "BROWSER="+shim)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, out, os.Stderr
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		return 0, fmt.Errorf("azrl: bridge: %w", err)
	}
	return 0, nil
}

// runBridge is bridgeRun with the child's exit status passed through.
func runBridge(args []string, out io.Writer) error {
	code, err := bridgeRun(args, out)
	if err != nil {
		return err
	}
	if code != 0 {
		shellExit(code)
	}
	return nil
}

func newBridgeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bridge <command> [args…]",
		Short: "Run any command with the browser bridge — sign-in pages open on your local browser",
		Long: `bridge runs a command with $BROWSER wired to azrl's smart shim, so a plain
CLI login (az, gcloud, gh, …) opens its sign-in page on your local machine
and gets its OAuth callback tunnelled back to this host. azrl never touches
where the command stores its session — use it to renew the native (default)
session that unmapped directories fall back to.

  azrl bridge az login --tenant velrada.com
  azrl bridge gcloud auth login`,
		SilenceUsage:       true,
		DisableFlagParsing: true, // everything after `bridge` belongs to the child command
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
				return cmd.Help()
			}
			return runBridge(args, cmd.OutOrStdout())
		},
	}
}

func init() { RootCmd.AddCommand(newBridgeCmd()) }
