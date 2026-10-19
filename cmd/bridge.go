package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/bridge"
)

// bridgeShimScript is the single-word $BROWSER target `azrl bridge` points a
// child command at. A bare executable path is the one convention every
// browser launcher handles (python's webbrowser needs `%s` for arguments and
// `&` to avoid blocking; gh shell-splits; GCM ignores $BROWSER entirely) —
// and the shim backgrounds the real bridge and returns immediately, so
// launchers that wait for $BROWSER to exit never block the OAuth callback
// they are about to serve. The background process must not hold the
// launcher's stdio (a launcher reading its pipe to EOF would hang), so its
// output — the paste line and any fallback notes — goes to logPath, which
// bridgeRun relays to the user's terminal.
func bridgeShimScript(bin, logPath string) string {
	return fmt.Sprintf("#!/bin/sh\nnohup %s __browser \"$1\" </dev/null >>%s 2>&1 &\n",
		bridge.ShellQuote(bin), bridge.ShellQuote(logPath))
}

// writeBridgeShim writes the shim and its (empty) output log into a private
// temp dir, returning both paths and a cleanup func. The dir can be removed
// as soon as the bridged command exits — the backgrounded __browser process
// no longer needs it.
func writeBridgeShim() (shim, logPath string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "azrl-bridge-")
	if err != nil {
		return "", "", nil, err
	}
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "azrl"
	}
	logPath = filepath.Join(dir, "browser.log")
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		os.RemoveAll(dir)
		return "", "", nil, err
	}
	shim = filepath.Join(dir, "azrl-browser")
	if err := os.WriteFile(shim, []byte(bridgeShimScript(self, logPath)), 0o755); err != nil {
		os.RemoveAll(dir)
		return "", "", nil, err
	}
	return shim, logPath, func() { os.RemoveAll(dir) }, nil
}

// bridgeNotices is where the backgrounded shim's output is relayed.
var bridgeNotices io.Writer = os.Stderr

// relayLog copies whatever is appended to path onto w until stop is called,
// then drains the rest. It is how the path-A paste line (and every "zero-paste
// unavailable" note) reaches the user while the bridged CLI waits for its
// callback.
func relayLog(path string, w io.Writer) (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		var off int64
		drain := func() {
			f, err := os.Open(path)
			if err != nil {
				return
			}
			defer f.Close()
			if _, err := f.Seek(off, io.SeekStart); err != nil {
				return
			}
			n, _ := io.Copy(w, f)
			off += n
		}
		t := time.NewTicker(150 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				drain()
				return
			case <-t.C:
				drain()
			}
		}
	}()
	return func() { close(done); <-finished }
}

// bridgeRun execs the command with $BROWSER wired to the shim, streaming its
// stdio. It returns the child's exit code (0 on success) so callers can react
// (e.g. gh default's switch→login fallback); a non-ExitError failure to start
// is the returned error. azrl never touches where the command stores its
// session — this is plumbing only (PAT-002).
//
// drop names extra variables to remove from the child's environment (see
// nativeEnvScrub), on top of the BROWSER the shim replaces.
func bridgeRun(args []string, out io.Writer, drop ...string) (int, error) {
	shim, logPath, cleanup, err := writeBridgeShim()
	if err != nil {
		return 0, fmt.Errorf("azrl: bridge: %w", err)
	}
	defer cleanup()
	stopRelay := relayLog(logPath, bridgeNotices)
	defer stopRelay()
	c := exec.Command(args[0], args[1:]...)
	c.Env = append(scrubEnv(os.Environ(), append([]string{"BROWSER"}, drop...)), "BROWSER="+shim)
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
func runBridge(args []string, out io.Writer, drop ...string) error {
	code, err := bridgeRun(args, out, drop...)
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
