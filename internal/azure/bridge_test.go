package azure

import (
	"os/exec"
	"testing"
	"time"
)

// newTestLogin starts cmd and wires up the waitErr channel exactly as LoginCapture does.
func newTestLogin(t *testing.T, cmd *exec.Cmd) *Login {
	t.Helper()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	lg := &Login{Cmd: cmd}
	lg.waitErr = make(chan error, 1)
	go func() { lg.waitErr <- cmd.Wait() }()
	return lg
}

func TestWaitForLoginSuccessAndTimeout(t *testing.T) {
	okLg := newTestLogin(t, exec.Command("true"))
	if err := WaitForLogin(okLg, 5*time.Second); err != nil {
		t.Fatalf("success: %v", err)
	}
	slowLg := newTestLogin(t, exec.Command("sleep", "10"))
	if err := WaitForLogin(slowLg, 200*time.Millisecond); err == nil {
		t.Fatal("expected timeout error")
	}
}
