package azure

import (
	"fmt"
	"time"
)

// WaitForLogin waits for the az login process (tracked in lg.waitErr) to finish
// within the given timeout. On timeout it kills the process, drains the channel,
// and returns an error. This must not call cmd.Wait() — the goroutine started by
// LoginCapture owns that.
func WaitForLogin(lg *Login, timeout time.Duration) error {
	select {
	case err := <-lg.waitErr:
		return err
	case <-time.After(timeout):
		_ = lg.Cmd.Process.Kill()
		<-lg.waitErr
		return fmt.Errorf("azrl: sign-in did not complete within %s", timeout)
	}
}
