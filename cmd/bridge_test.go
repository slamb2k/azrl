package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestBridgeShimScriptShape(t *testing.T) {
	s := bridgeShimScript("/usr/local/bin/azrl", "/tmp/azrl-bridge-x/browser.log")
	if !strings.HasPrefix(s, "#!/bin/sh\n") {
		t.Fatalf("shim must be a sh script: %q", s)
	}
	for _, want := range []string{"nohup", `'/usr/local/bin/azrl' __browser "$1"`, ">>'/tmp/azrl-bridge-x/browser.log' 2>&1", "</dev/null", "&"} {
		if !strings.Contains(s, want) {
			t.Fatalf("shim missing %q:\n%s", want, s)
		}
	}
}

// The bridged child sees $BROWSER pointing at an executable single-word shim
// that routes through __browser — no %s/& conventions required of the child.
func TestBridgeWiresExecutableShim(t *testing.T) {
	t.Setenv("BROWSER", "stale-value")
	var out bytes.Buffer
	err := runBridge([]string{"sh", "-c",
		`test -x "$BROWSER" && grep -q __browser "$BROWSER" && echo "shim ok: $BROWSER"`}, &out)
	if err != nil {
		t.Fatalf("runBridge: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "shim ok: ") || strings.Contains(out.String(), "stale-value") {
		t.Fatalf("child should see the shim path in $BROWSER, got %q", out.String())
	}
}

// The child's exit status passes through (same seam as azrl shell).
func TestBridgeExitStatusPassthrough(t *testing.T) {
	got := -1
	old := shellExit
	shellExit = func(code int) { got = code }
	t.Cleanup(func() { shellExit = old })
	var out bytes.Buffer
	if err := runBridge([]string{"sh", "-c", "exit 3"}, &out); err != nil {
		t.Fatalf("exit status should pass through, not error: %v", err)
	}
	if got != 3 {
		t.Fatalf("exit code = %d, want 3", got)
	}
}

// A missing command is a real error, not an exit-status passthrough.
func TestBridgeMissingCommandErrors(t *testing.T) {
	var out bytes.Buffer
	if err := runBridge([]string{"azrl-definitely-not-a-command"}, &out); err == nil {
		t.Fatal("missing command should error")
	}
	if _, err := os.Stat("azrl-definitely-not-a-command"); err == nil {
		t.Fatal("test invariant broken")
	}
}

// TestBridgeRelaysShimOutput is the regression for the paste line vanishing
// under `azrl bridge`: whatever the backgrounded __browser writes (the path-A
// paste line, fallback notes) must reach the user, not /dev/null.
func TestBridgeRelaysShimOutput(t *testing.T) {
	var notices bytes.Buffer
	old := bridgeNotices
	bridgeNotices = &notices
	t.Cleanup(func() { bridgeNotices = old })
	// Stand-in for the shim's background process: append to the log the
	// shim script names, the way __browser's stdout would.
	child := `echo "paste this on your LOCAL machine: ssh -fNL 1:localhost:1 vm" >> "$(dirname "$BROWSER")/browser.log"; sleep 0.4`
	var out bytes.Buffer
	if err := runBridge([]string{"sh", "-c", child}, &out); err != nil {
		t.Fatalf("runBridge: %v\n%s", err, out.String())
	}
	if !strings.Contains(notices.String(), "paste this on your LOCAL machine") {
		t.Fatalf("paste line not relayed; notices=%q", notices.String())
	}
}
