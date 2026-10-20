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
	want := `ssh -fNL 38149:localhost:38149 vm-always && wslview 'https://login/x?y=z'`
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

func TestOpenURLLocalRunsBrowserCmd(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "browser.log")
	script := "#!/usr/bin/env bash\necho \"$*\" >> \"" + log + "\"\n"
	if err := os.WriteFile(filepath.Join(bin, "mybrowser"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	g := config.Global{BrowserCmd: "mybrowser"} // BrowserHost empty ⇒ local
	if err := OpenURL(g, "https://portal.azure.com/#@acme.com"); err != nil {
		t.Fatal(err)
	}
	// LaunchLocal starts the command asynchronously; poll briefly for the log.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if b, _ := os.ReadFile(log); strings.Contains(string(b), "https://portal.azure.com/#@acme.com") {
			return
		}
		if time.Now().After(deadline) {
			b, _ := os.ReadFile(log)
			t.Fatalf("browser cmd never received the URL; log: %s", b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestOpenURLRemoteGoesOverSSH(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "ssh.log")
	script := "#!/usr/bin/env bash\necho \"$*\" >> \"" + log + "\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	g := config.Global{BrowserHost: "pc", BrowserCmd: "wslview"}
	if err := OpenURL(g, "https://acme.awsapps.com/start"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	got := string(b)
	if !strings.Contains(got, "pc") || !strings.Contains(got, "wslview 'https://acme.awsapps.com/start'") {
		t.Fatalf("remote launch not over ssh with the browser cmd; log: %s", got)
	}
}

// TestBrowserLineQuotesHostileURL proves a URL can never break out of its
// quoting: a ' or $(…) in it reaches the browser as literal text, not code.
func TestBrowserLineQuotesHostileURL(t *testing.T) {
	bin := t.TempDir()
	pwned := filepath.Join(bin, "pwned")
	marker := filepath.Join(bin, "opened")
	browser := filepath.Join(bin, "fakebrowser")
	if err := os.WriteFile(browser, []byte("#!/usr/bin/env bash\nprintf '%s' \"$1\" > \""+marker+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	url := "https://x/?a='; touch " + pwned + "; echo '$(touch " + pwned + ")`touch " + pwned + "`"
	if err := LaunchLocal(browser, url); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if b, err := os.ReadFile(marker); err == nil && len(b) > 0 {
			if string(b) != url {
				t.Fatalf("browser got %q, want the URL verbatim %q", b, url)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("browser never launched")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(pwned); err == nil {
		t.Fatal("URL content was executed by the shell")
	}
}

func TestBridgeTunnelExitsOnForwardFailure(t *testing.T) {
	log := fakeTunnelSSH(t, "sleep 2; exit 0")
	g := config.Global{BrowserHost: "pc", BrowserCmd: "wslview", VMSSHHost: "vm"}
	tun, _, err := Bridge("40404", "https://login/x", g, false)
	if err != nil || tun == nil {
		t.Fatalf("expected a live tunnel: tun=%v err=%v", tun, err)
	}
	defer tun.Process.Kill()
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "-o ExitOnForwardFailure=yes -N -R 40404:localhost:40404 pc") {
		t.Fatalf("tunnel must set ExitOnForwardFailure; log: %s", b)
	}
}

// TestLaunchRemoteDoesNotBlockOnForegroundBrowser proves a browser command
// that never returns (ssh held open) releases the caller after launchWait.
func TestLaunchRemoteDoesNotBlockOnForegroundBrowser(t *testing.T) {
	log := fakeTunnelSSH(t, "exit 0")
	// Any non -R invocation (the browser launch) hangs.
	b, _ := os.ReadFile(filepath.Join(filepath.Dir(log), "ssh"))
	os.WriteFile(filepath.Join(filepath.Dir(log), "ssh"), []byte(strings.Replace(string(b), "\nexit 0\n", "\nsleep 30\n", 1)), 0o755)
	old := launchWait
	launchWait = 200 * time.Millisecond
	t.Cleanup(func() { launchWait = old })
	start := time.Now()
	if err := LaunchRemote("pc", "chrome", "https://x"); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("LaunchRemote blocked on a foreground browser")
	}
	got, _ := os.ReadFile(log)
	if !strings.Contains(string(got), "-o BatchMode=yes -o ConnectTimeout=5 pc chrome 'https://x'") {
		t.Fatalf("launch must be batch-mode with a connect timeout; log: %s", got)
	}
}

// fakeTunnelSSH installs an ssh shim that logs its args; a -R invocation runs
// onTunnel, anything else exits 0. Returns the log path.
func fakeTunnelSSH(t *testing.T, onTunnel string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "ssh.log")
	script := "#!/usr/bin/env bash\necho \"$*\" >> \"" + log + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && { " + onTunnel + "; }; done\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// TestLaunchLocalReportsFailedLauncher proves a launcher that exits non-zero
// (wslview with broken interop, a missing binary) surfaces as an error carrying
// its last line of output instead of being reported as an opened browser.
func TestLaunchLocalReportsFailedLauncher(t *testing.T) {
	bin := t.TempDir()
	browser := filepath.Join(bin, "badbrowser")
	if err := os.WriteFile(browser, []byte("#!/usr/bin/env bash\necho noise >&2\necho 'interop is disabled' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := LaunchLocal(browser, "https://login/x")
	if err == nil || !strings.Contains(err.Error(), "interop is disabled") {
		t.Fatalf("want the launcher's error, got %v", err)
	}
}

// TestLaunchLocalDoesNotBlockOnForegroundBrowser proves a browser that stays in
// the foreground releases the caller after localLaunchWait.
func TestLaunchLocalDoesNotBlockOnForegroundBrowser(t *testing.T) {
	bin := t.TempDir()
	browser := filepath.Join(bin, "slowbrowser")
	if err := os.WriteFile(browser, []byte("#!/usr/bin/env bash\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := localLaunchWait
	localLaunchWait = 200 * time.Millisecond
	t.Cleanup(func() { localLaunchWait = old })
	start := time.Now()
	if err := LaunchLocal(browser, "https://login/x"); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("LaunchLocal blocked on a foreground browser")
	}
}

// TestBridgeLocalLaunchFailureReturnsURL proves local mode hands the URL back
// when the launcher fails, so the caller can ask the user to open it.
func TestBridgeLocalLaunchFailureReturnsURL(t *testing.T) {
	g := config.Global{BrowserCmd: "false"}
	tun, paste, err := Bridge("40404", "https://login/x?y=z", g, false)
	if err != nil || tun != nil {
		t.Fatalf("tun=%v err=%v", tun, err)
	}
	if paste != "https://login/x?y=z" {
		t.Fatalf("want the URL back on a failed launch, got %q", paste)
	}
}

// TestLocalNoteAlwaysShowsURL proves the sign-in URL is printed whether or not
// the launch reported success — a launcher can exit 0 and show nothing.
func TestLocalNoteAlwaysShowsURL(t *testing.T) {
	url := "https://login/x?y=z"
	ok := LocalNote("azrl", "wslview", url, "")
	if !strings.Contains(ok, "browser opened locally (wslview)") || !strings.Contains(ok, url) {
		t.Fatalf("success note: %q", ok)
	}
	failed := LocalNote("azrl", "wslview", url, url)
	if strings.Contains(failed, "opened") || !strings.Contains(failed, url) {
		t.Fatalf("failure note: %q", failed)
	}
}
