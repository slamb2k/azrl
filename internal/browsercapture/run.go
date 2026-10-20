package browsercapture

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/slamb2k/azrl/internal/bridge"
	"github.com/slamb2k/azrl/internal/config"
)

// Run classifies url and connects it to the user's local browser. Loopback OAuth
// URLs reuse the SSH bridge: path B opens a reverse tunnel for the callback port
// (held for the auth window `hold`, then torn down); path A returns the paste
// line. Device/plain URLs are relayed — opened directly on the laptop (path B)
// or returned for the user to open (path A). The returned string is a
// user-facing message ("" when the action completed silently over SSH).
func Run(url string, g config.Global, forcePaste bool, hold time.Duration) (string, error) {
	if Classify(url) == Loopback {
		port := ParseCallbackPort(url)
		tunnel, paste, err := bridge.Bridge(port, url, g, forcePaste)
		if err != nil {
			return "", err
		}
		if tunnel != nil {
			time.Sleep(hold)
			_ = tunnel.Process.Kill()
			return "", nil
		}
		if g.IsLocal() && paste != "" {
			return localOpenLine(paste), nil
		}
		return paste, nil
	}
	return relayDevice(url, g, forcePaste), nil
}

// relayDevice opens a device/plain URL on the laptop. Local mode launches the
// browser on this host; path B (BrowserHost reachable) launches it over SSH;
// path A returns a local-open line.
func relayDevice(url string, g config.Global, forcePaste bool) string {
	if g.IsLocal() {
		if err := bridge.LaunchLocal(g.BrowserCmd, url); err != nil {
			return fmt.Sprintf("could not open the browser with %q: %v\n", g.BrowserCmd, err) + localOpenLine(url)
		}
		return ""
	}
	if !forcePaste && reachable(g.BrowserHost) {
		_ = bridge.LaunchRemote(g.BrowserHost, g.BrowserCmd, url)
		return ""
	}
	return fmt.Sprintf("open this URL on your LOCAL machine:\n\n  %s", bridge.BrowserLine(g.BrowserCmd, url))
}

// reachable reports whether host answers a batch-mode SSH probe.
func reachable(host string) bool {
	return exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", host, "true").Run() == nil
}

// localOpenLine asks the user to open url themselves after a local launch
// failed — the callback is on this machine, so any browser here will do.
func localOpenLine(url string) string {
	return fmt.Sprintf("open this URL in your browser to sign in:\n\n  %s", url)
}
