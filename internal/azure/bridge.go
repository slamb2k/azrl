package azure

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/slamb2k/azrl/internal/config"
)

// PasteLine is the one-line command the user runs on their LOCAL machine.
func PasteLine(port, vmHost, browserCmd, url string) string {
	return fmt.Sprintf("ssh -fNL %s:localhost:%s %s && %s %q", port, port, vmHost, browserCmd, url)
}

// Bridge connects the local browser to the VM's callback port. Path B (default):
// if LocalHost is SSH-reachable, open a reverse tunnel and launch the browser
// there, returning the tunnel command (kill it during teardown). Path A
// (forcePaste or unreachable): return the paste line for the user.
func Bridge(port, url string, g config.Global, forcePaste bool) (*exec.Cmd, string, error) {
	reachable := false
	if !forcePaste {
		probe := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", g.LocalHost, "true")
		reachable = probe.Run() == nil
	}
	if !reachable {
		return nil, PasteLine(port, g.VMHost, g.LocalBrowserCmd, url), nil
	}
	tunnel := exec.Command("ssh", "-N", "-R", fmt.Sprintf("%s:localhost:%s", port, port), g.LocalHost)
	if err := tunnel.Start(); err != nil {
		return nil, PasteLine(port, g.VMHost, g.LocalBrowserCmd, url), nil
	}
	time.Sleep(500 * time.Millisecond)
	if tunnel.ProcessState != nil && tunnel.ProcessState.Exited() {
		return nil, PasteLine(port, g.VMHost, g.LocalBrowserCmd, url), nil
	}
	_ = exec.Command("ssh", g.LocalHost, fmt.Sprintf("%s '%s'", g.LocalBrowserCmd, url)).Run()
	return tunnel, "", nil
}

// WaitForLogin waits for cmd with a deadline; on timeout it kills the process
// and returns an error.
func WaitForLogin(cmd *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("azrl: sign-in did not complete within %s", timeout)
	}
}
