// Package browsercapture holds the logic behind the hidden __browser-capture
// shim: whatever URL a tool tries to open is handed to the shim, which records
// it so the login flow can parse the OAuth callback port. The smart
// classify-and-bridge behaviour builds on this capture primitive.
package browsercapture

import (
	"fmt"
	"os"
)

// Capture records url to capfile (0600). It errors when capfile is empty.
func Capture(capfile, url string) error {
	if capfile == "" {
		return fmt.Errorf("browsercapture: capfile not set")
	}
	return os.WriteFile(capfile, []byte(url), 0o600)
}
