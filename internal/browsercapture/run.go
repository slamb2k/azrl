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
		return paste, nil
	}
	return relayDevice(url, g, forcePaste), nil
}

// relayDevice opens a device/plain URL on the laptop. Local mode launches the
// browser on this host; path B (BrowserHost reachable) launches it over SSH;
// path A returns a local-open line.
func relayDevice(url string, g config.Global, forcePaste bool) string {
	if g.IsLocal() {
		_ = bridge.LaunchLocal(g.BrowserCmd, url)
		return ""
	}
	if !forcePaste && reachable(g.BrowserHost) {
		_ = exec.Command("ssh", g.BrowserHost, fmt.Sprintf("%s '%s'", g.BrowserCmd, url)).Run()
		return ""
	}
	return fmt.Sprintf("open this URL on your LOCAL machine:\n\n  %s %s", g.BrowserCmd, url)
}

// reachable reports whether host answers a batch-mode SSH probe.
func reachable(host string) bool {
	return exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", host, "true").Run() == nil
}
