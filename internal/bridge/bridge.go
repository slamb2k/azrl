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
	"time"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/envdetect"
)

// PasteLine is the one-line command the user runs on their LOCAL machine.
func PasteLine(port, vmHost, browserCmd, url string) string {
	return fmt.Sprintf("ssh -fNL %s:localhost:%s %s && %s %q", port, port, vmHost, browserCmd, url)
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
	return exec.Command("sh", "-c", fmt.Sprintf("%s '%s'", browserCmd, url)).Start()
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
		return nil, PasteLine(port, vmHost, g.BrowserCmd, url), nil
	}
	tunnel := exec.Command("ssh", "-N", "-R", fmt.Sprintf("%s:localhost:%s", port, port), g.BrowserHost)
	if err := tunnel.Start(); err != nil {
		return nil, PasteLine(port, vmHost, g.BrowserCmd, url), nil
	}
	// Detect tunnels that die immediately (port conflict, auth failure, etc.).
	// ProcessState is only set after Wait(), so we use a goroutine + select.
	tunnelDone := make(chan error, 1)
	go func() { tunnelDone <- tunnel.Wait() }()
	select {
	case <-tunnelDone:
		// Tunnel exited within the liveness window — fall back to paste.
		return nil, PasteLine(port, vmHost, g.BrowserCmd, url), nil
	case <-time.After(500 * time.Millisecond):
		// Tunnel is still alive — open the remote browser and return it.
	}
	_ = exec.Command("ssh", g.BrowserHost, fmt.Sprintf("%s '%s'", g.BrowserCmd, url)).Run()
	return tunnel, "", nil
}
