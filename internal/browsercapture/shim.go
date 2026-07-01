package browsercapture

import "fmt"

// XdgOpenShimScript returns the contents of a wrapper to install on the session
// PATH as `xdg-open`, ahead of the real one. Git Credential Manager ignores
// $BROWSER on Linux and execs xdg-open directly, so shadowing xdg-open is how
// its loopback authorize URL reaches the shim. gh, which honors $BROWSER, is
// wired separately via BROWSER=<bin> __browser.
func XdgOpenShimScript(bin string) string {
	return fmt.Sprintf("#!/usr/bin/env bash\nexec %q __browser \"$@\"\n", bin)
}
