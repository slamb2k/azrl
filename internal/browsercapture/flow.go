package browsercapture

import (
	"fmt"
	"os"
	"time"
)

// CaptureCommand returns the BROWSER value: env AZRL_CAPTURE override, else the
// running binary invoked as its hidden __browser-capture subcommand.
func CaptureCommand() string {
	if c := os.Getenv("AZRL_CAPTURE"); c != "" {
		return c
	}
	self, err := os.Executable()
	if err != nil {
		self = "azrl"
	}
	return self + " __browser-capture"
}

// LoginTimeout returns AZRL_LOGIN_TIMEOUT seconds (default 180).
func LoginTimeout() time.Duration {
	if v := os.Getenv("AZRL_LOGIN_TIMEOUT"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 180 * time.Second
}
