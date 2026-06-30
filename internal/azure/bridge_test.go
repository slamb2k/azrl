package azure

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/slamb2k/azrl/internal/config"
)

func TestPasteLine(t *testing.T) {
	got := PasteLine("38149", "vm-always", "wslview", "https://login/x?y=z")
	want := `ssh -fNL 38149:localhost:38149 vm-always && wslview "https://login/x?y=z"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBridgePathA(t *testing.T) {
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm-always"}
	tun, paste, err := Bridge("40404", "https://login/x", g, true)
	if err != nil || tun != nil {
		t.Fatalf("forced paste should not tunnel: tun=%v err=%v", tun, err)
	}
	if paste == "" || !contains(paste, "ssh -fNL 40404:localhost:40404 vm-always") {
		t.Fatalf("paste=%q", paste)
	}
}

func TestBridgePathB(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "ssh.log")
	// ssh shim: reachability + browser cmd succeed; -R reverse tunnel stays up.
	sshScript := "#!/usr/bin/env bash\necho \"$*\" >> \"" + log + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && { sleep 2; exit 0; }; done\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(sshScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm-always"}
	tun, paste, err := Bridge("40404", "https://login/x", g, false)
	if err != nil || tun == nil || paste != "" {
		t.Fatalf("path B: tun=%v paste=%q err=%v", tun, paste, err)
	}
	defer func() { _ = tun.Process.Kill() }()
	b, _ := os.ReadFile(log)
	if !contains(string(b), "-R 40404:localhost:40404 pc") || !contains(string(b), "wslview") {
		t.Fatalf("ssh log missing tunnel/browser: %s", b)
	}
}

func TestBridgePathBDeadTunnel(t *testing.T) {
	bin := t.TempDir()
	// ssh shim: probe (no -R flag) succeeds; tunnel (-R flag) exits immediately nonzero.
	sshScript := "#!/usr/bin/env bash\nfor a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && exit 1; done\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(sshScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm-always"}
	tun, paste, err := Bridge("40404", "https://login/x", g, false)
	if err != nil {
		t.Fatalf("dead tunnel: unexpected error: %v", err)
	}
	if tun != nil {
		t.Fatalf("dead tunnel: expected nil cmd, got %v", tun)
	}
	if paste == "" || !contains(paste, "ssh -fNL 40404:localhost:40404 vm-always") {
		t.Fatalf("dead tunnel: expected paste fallback, got %q", paste)
	}
}

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
