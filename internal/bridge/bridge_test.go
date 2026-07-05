package bridge

import (
	"os"
	"path/filepath"
	"strings"
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

func TestVMHost(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "")
	if h, ok := VMHost(config.Global{VMSSHHost: "vm-set"}); h != "vm-set" || !ok {
		t.Fatalf("explicit VM_SSH_HOST: got %q ok=%v", h, ok)
	}
	t.Setenv("SSH_CONNECTION", "198.51.100.2 51000 203.0.113.10 22")
	if h, ok := VMHost(config.Global{}); h != "203.0.113.10" || !ok {
		t.Fatalf("derived: got %q ok=%v", h, ok)
	}
	t.Setenv("SSH_CONNECTION", "")
	if h, ok := VMHost(config.Global{}); h != "<your-vm-host>" || ok {
		t.Fatalf("placeholder: got %q ok=%v", h, ok)
	}
}

func TestBridgePathA(t *testing.T) {
	g := config.Global{BrowserHost: "pc", BrowserCmd: "wslview", VMSSHHost: "vm-always"}
	tun, paste, err := Bridge("40404", "https://login/x", g, true)
	if err != nil || tun != nil {
		t.Fatalf("forced paste should not tunnel: tun=%v err=%v", tun, err)
	}
	if paste == "" || !strings.Contains(paste, "ssh -fNL 40404:localhost:40404 vm-always") {
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
	g := config.Global{BrowserHost: "pc", BrowserCmd: "wslview", VMSSHHost: "vm-always"}
	tun, paste, err := Bridge("40404", "https://login/x", g, false)
	if err != nil || tun == nil || paste != "" {
		t.Fatalf("path B: tun=%v paste=%q err=%v", tun, paste, err)
	}
	defer func() { _ = tun.Process.Kill() }()
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "-R 40404:localhost:40404 pc") || !strings.Contains(string(b), "wslview") {
		t.Fatalf("ssh log missing tunnel/browser: %s", b)
	}
}

func TestBridgeLocalMode(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "opened")
	// Fake browser: record the URL it was asked to open.
	browser := filepath.Join(bin, "fakebrowser")
	script := "#!/usr/bin/env bash\necho \"$1\" > \"" + marker + "\"\n"
	if err := os.WriteFile(browser, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// ssh shim that flags any invocation — local mode must not touch SSH.
	sshFlag := filepath.Join(bin, "ssh-called")
	ssh := "#!/usr/bin/env bash\ntouch \"" + sshFlag + "\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(ssh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	g := config.Global{BrowserHost: "localhost", BrowserCmd: browser}
	tun, paste, err := Bridge("40404", "https://login/x?y=z", g, false)
	if err != nil || tun != nil || paste != "" {
		t.Fatalf("local mode: tun=%v paste=%q err=%v", tun, paste, err)
	}
	// LaunchLocal is async (Start); wait briefly for the browser to run.
	var got []byte
	for i := 0; i < 50; i++ {
		if b, e := os.ReadFile(marker); e == nil {
			got = b
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(string(got), "https://login/x?y=z") {
		t.Fatalf("browser not launched with url, marker=%q", got)
	}
	if _, err := os.Stat(sshFlag); err == nil {
		t.Fatal("local mode must not invoke ssh")
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
	g := config.Global{BrowserHost: "pc", BrowserCmd: "wslview", VMSSHHost: "vm-always"}
	tun, paste, err := Bridge("40404", "https://login/x", g, false)
	if err != nil {
		t.Fatalf("dead tunnel: unexpected error: %v", err)
	}
	if tun != nil {
		t.Fatalf("dead tunnel: expected nil cmd, got %v", tun)
	}
	if paste == "" || !strings.Contains(paste, "ssh -fNL 40404:localhost:40404 vm-always") {
		t.Fatalf("dead tunnel: expected paste fallback, got %q", paste)
	}
}
