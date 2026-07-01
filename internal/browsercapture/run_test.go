package browsercapture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/slamb2k/azrl/internal/config"
)

func fakeSSH(t *testing.T, tunnelStaysUp bool) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "ssh.log")
	stay := "exit 0"
	if tunnelStaysUp {
		stay = "sleep 2; exit 0"
	}
	script := "#!/usr/bin/env bash\necho \"$*\" >> \"" + log + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && { " + stay + "; }; done\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestRunDeviceRelayPathB(t *testing.T) {
	log := fakeSSH(t, false)
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm"}
	msg, err := Run("https://github.com/login/device", g, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if msg != "" {
		t.Fatalf("path B device relay should not print paste; got %q", msg)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "pc") || !strings.Contains(string(b), "wslview") {
		t.Fatalf("ssh relay missing host/browser: %s", b)
	}
}

func TestRunDeviceRelayPathA(t *testing.T) {
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm"}
	msg, err := Run("https://github.com/login/device", g, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "https://github.com/login/device") || !strings.Contains(msg, "wslview") {
		t.Fatalf("path A should print a local-open line; got %q", msg)
	}
}

func TestRunLoopbackPathA(t *testing.T) {
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm"}
	url := "https://h/o?redirect_uri=http://127.0.0.1:52001/"
	msg, err := Run(url, g, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "ssh -fNL 52001:localhost:52001 vm") {
		t.Fatalf("loopback path A should print the -L paste line; got %q", msg)
	}
}

func TestRunLoopbackPathBHoldsTunnel(t *testing.T) {
	log := fakeSSH(t, true)
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm"}
	url := "https://h/o?redirect_uri=http://127.0.0.1:52001/"
	start := time.Now()
	msg, err := Run(url, g, false, 150*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if msg != "" {
		t.Fatalf("path B loopback should tunnel silently; got %q", msg)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatal("Run should hold the tunnel for the auth window")
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "-R 52001:localhost:52001 pc") {
		t.Fatalf("reverse tunnel not opened: %s", b)
	}
}
