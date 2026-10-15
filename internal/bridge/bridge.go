// Package bridge connects a VM's OAuth callback port to the user's local
// browser over SSH. It is provider-agnostic: both the Azure login flow and the
// GitHub loopback shim reuse it. Path B (zero-paste) opens an SSH reverse tunnel
// and launches the local browser; path A (paste) prints a one-line command for
// the user to run locally.
package bridge

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/envdetect"
)

// PasteLine is the one-line command the user runs on their LOCAL machine. The
// URL is single-quoted so nothing in it ($(…), backticks, quotes) is expanded
// by the user's shell when they paste it.
func PasteLine(port, vmHost, browserCmd, url string) string {
	return fmt.Sprintf("ssh -fNL %s:localhost:%s %s && %s", port, port, vmHost, BrowserLine(browserCmd, url))
}

// ShellQuote wraps v in POSIX single quotes, escaping any embedded single
// quote, so the shell treats it as one literal word.
func ShellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// BrowserLine is the shell command that opens url with browserCmd. browserCmd
// is trusted user config (it may carry its own arguments); url is untrusted —
// it comes from whatever CLI invoked $BROWSER — so it is always quoted.
func BrowserLine(browserCmd, url string) string {
	return browserCmd + " " + ShellQuote(url)
}

// launchWait bounds how long a remote browser launch may hold the caller. A
// BROWSER_CMD that stays in the foreground (e.g. the first Chrome window)
// keeps ssh open until the browser exits; past this window the launch is
// treated as done and ssh is left running detached.
var launchWait = 10 * time.Second

// LaunchRemote runs the browser command for url on host over SSH. BatchMode
// and ConnectTimeout keep an unreachable or password-prompting host from
// hanging the caller; a launch still running after launchWait returns nil.
func LaunchRemote(host, browserCmd, url string) error {
	c := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", host, BrowserLine(browserCmd, url))
	if err := c.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(launchWait):
		return nil
	}
}

// VMHost resolves the VM's SSH name for the path-A paste line: VM_SSH_HOST if
// set, else the server IP derived from $SSH_CONNECTION, else the literal
// "<your-vm-host>" placeholder. ok is false only in the placeholder case, so the
// caller can print a hint to set VM_SSH_HOST.
func VMHost(g config.Global) (host string, ok bool) {
	if g.VMSSHHost != "" {
		return g.VMSSHHost, true
	}
	if h := envdetect.DeriveVMHost(os.Getenv("SSH_CONNECTION")); h != "" {
		return h, true
	}
	return "<your-vm-host>", false
}

// LaunchLocal opens url on the current machine using browserCmd, with no SSH.
// Used in local mode, where azrl runs on the same host as the browser and the
// OAuth callback loops back over localhost directly. Best-effort and detached:
// browserCmd may carry its own arguments, so it runs through the shell.
func LaunchLocal(browserCmd, url string) error {
	return exec.Command("sh", "-c", BrowserLine(browserCmd, url)).Start()
}

// Bridge connects the local browser to the VM's callback port. Path B (default):
// if LocalHost is SSH-reachable, open a reverse tunnel and launch the browser
// there, returning the tunnel command (kill it during teardown). Path A
// (forcePaste or unreachable): return the paste line for the user.
func Bridge(port, url string, g config.Global, forcePaste bool) (*exec.Cmd, string, error) {
	if g.IsLocal() {
		// Local mode: azrl and the browser share this host, so the callback
		// loops back over localhost — no tunnel, no paste line.
		_ = LaunchLocal(g.BrowserCmd, url)
		return nil, "", nil
	}
	vmHost, _ := VMHost(g)
	reachable := false
	if !forcePaste {
		probe := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", g.BrowserHost, "true")
		reachable = probe.Run() == nil
	}
	if !reachable {
		if !forcePaste {
			fmt.Fprintf(os.Stderr, "azrl: zero-paste unavailable — ssh probe to %s failed (BatchMode, 5s timeout); falling back to the paste line\n", g.BrowserHost)
		}
		return nil, PasteLine(port, vmHost, g.BrowserCmd, url), nil
	}
	// ExitOnForwardFailure makes ssh exit when the remote port is already bound
	// (a stale tunnel, a concurrent login) instead of warning and staying up —
	// which is what lets the liveness check below catch the conflict.
	tunnel := exec.Command("ssh", "-o", "ExitOnForwardFailure=yes", "-N", "-R", fmt.Sprintf("%s:localhost:%s", port, port), g.BrowserHost)
	if err := tunnel.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "azrl: zero-paste unavailable — could not start the ssh tunnel: %v; falling back to the paste line\n", err)
		return nil, PasteLine(port, vmHost, g.BrowserCmd, url), nil
	}
	// Detect tunnels that die immediately (port conflict, auth failure, etc.).
	// ProcessState is only set after Wait(), so we use a goroutine + select.
	tunnelDone := make(chan error, 1)
	go func() { tunnelDone <- tunnel.Wait() }()
	select {
	case <-tunnelDone:
		// Tunnel exited within the liveness window — fall back to paste.
		fmt.Fprintf(os.Stderr, "azrl: zero-paste unavailable — the ssh tunnel to %s exited immediately (port %s already forwarded, or ssh auth failed); falling back to the paste line\n", g.BrowserHost, port)
		return nil, PasteLine(port, vmHost, g.BrowserCmd, url), nil
	case <-time.After(500 * time.Millisecond):
		// Tunnel is still alive — open the remote browser and return it.
	}
	_ = LaunchRemote(g.BrowserHost, g.BrowserCmd, url)
	return tunnel, "", nil
}

// OpenURL opens a plain URL in the configured browser — the login bridge's
// launch primitives without the OAuth callback tunnel (a console link has no
// port to forward). Local mode launches directly; remote runs the browser
// command on BrowserHost over SSH.
func OpenURL(g config.Global, url string) error {
	if g.IsLocal() {
		return LaunchLocal(g.BrowserCmd, url)
	}
	return LaunchRemote(g.BrowserHost, g.BrowserCmd, url)
}
